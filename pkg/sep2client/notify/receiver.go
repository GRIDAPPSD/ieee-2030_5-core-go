// Package notify provides the library-grade inbound HTTPS Notification
// receiver for IEEE 2030.5 / CSIP CORE-018 subscription/notification flows.
//
// The receiver is a spec-conformant HTTP listener that accepts POST /notify
// requests, decodes the IEEE 2030.5 Notification XML body, and dispatches
// the result to a caller-supplied Dispatcher. It carries no consumer policy:
// routing, cache updates, and state-machine integration belong in the
// consuming application.
//
// mTLS posture mirrors the SEP2 server: ClientAuth = RequireAnyClientCert
// plus a VerifyPeerCertificate hook that tolerates the critical
// HardwareModuleName SAN (OID 1.3.6.1.5.5.7.8.4) which stdlib x509 cannot
// parse.
package notify

import (
	"context"
	"crypto/x509"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
	sepTLS "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls"
	gotls "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls/gotls"
)

// gotlsConnKey is the context key under which the accepted *gotls.Conn is
// stored for each connection. The handler reads it to access the
// post-handshake ConnectionState (including PeerCertificates) because
// gotls.Conn is not *crypto/tls.Conn and http.Server does not populate
// req.TLS for it.
type gotlsConnKey struct{}

// contentTypeSEPXML is the mandatory content type for IEEE 2030.5 resource
// payloads per IEEE 2030.5 section 10 / CSIP section 6.6.
const contentTypeSEPXML = "application/sep+xml"

// maxBodyBytes caps the Notification POST body to prevent OOM from a buggy
// or hostile server. IEEE 2030.5 Notification resources are typically under
// 2 KiB; 64 KiB is generous headroom.
const maxBodyBytes = 64 * 1024

// readHeaderTimeout guards against slowloris-style attacks where a peer
// dribbles header bytes. Matches the conservative posture used elsewhere in
// the project.
const readHeaderTimeout = 5 * time.Second

// shutdownTimeout is the internal cap on the graceful-shutdown phase inside
// Stop. See the Stop docstring for the interaction with the caller-supplied
// context.
const shutdownTimeout = 5 * time.Second

// Dispatcher is the callback invoked for every well-formed Notification POST.
//
// peerCert is the verified leaf certificate presented by the SEP2 server
// during the mTLS handshake. It is never nil: the mTLS posture rejects any
// request that does not carry a validated client certificate. Consumers can
// use peerCert to authorize the notification source, for example by
// extracting LFDI or SFDI from the HardwareModuleName SAN, before acting on
// the notification content.
//
// The dispatcher runs synchronously inside the /notify handler: keep it
// cheap. Heavy work (HTTP GETs to re-fetch a changed resource, state-machine
// ticks) belongs in a goroutine the dispatcher spawns itself, with a context
// tied to the receiver's lifetime.
//
// ctx is the per-request context; cancellation propagates to any goroutines
// the dispatcher spawns.
type Dispatcher func(ctx context.Context, peerCert *x509.Certificate, n sep2.Notification)

// Noop is a no-op Dispatcher that silently discards every well-formed
// Notification. Use it as the Config.Dispatcher default when no consumer is
// wired yet; it emits nothing and does not touch the process-global logger.
func Noop(_ context.Context, _ *x509.Certificate, _ sep2.Notification) {}

// Config carries the inputs NewReceiver needs.
//
// CertFile, KeyFile, and CAFile point at the PEM files for the device cert.
// The device cert acts as the listener's server cert; the CA pool provides
// the trust anchor for validating the SEP2 server's client cert during the
// mTLS handshake.
//
// ListenAddr is the bind address (e.g. "127.0.0.1:0" for a random port). An
// empty string disables the receiver: NewReceiver returns (nil, nil) in that
// case, which the caller treats as "subscription flow disabled, fall back to
// polling."
//
// Dispatcher is the per-notification callback. Nil substitutes Noop so
// callers can wire the listener and defer dispatcher selection.
//
// Logger, if non-nil, receives diagnostic messages from the receiver
// (currently: dispatcher panic recovery). A nil Logger is silent. Refused
// TLS handshakes are logged through the standard log package, where
// net/http logs its own server errors.
type Config struct {
	CertFile   string
	KeyFile    string
	CAFile     string
	ListenAddr string
	Dispatcher Dispatcher
	Logger     *slog.Logger
}

// Receiver owns the inbound HTTPS listener and the /notify handler.
//
// Lifecycle: NewReceiver -> Start -> Stop. Start binds the listener so
// Addr returns the bound address before any goroutine spawns, then runs
// the http.Server in a goroutine the caller's Stop can join.
type Receiver struct {
	tlsCfg     *gotls.Config
	listenAddr string
	dispatcher Dispatcher
	logger     *slog.Logger

	mu     sync.Mutex
	tlsL   net.Listener
	srv    *http.Server
	doneCh chan error
}

// ErrAlreadyStarted is returned by Start when the receiver is already
// running. Calling Start twice is a programmer error; the sentinel is
// exported so callers can match against it precisely.
var ErrAlreadyStarted = errors.New("notify receiver: already started")

// ErrNotStarted is returned by Addr and Stop when the receiver has never
// been started, or has already been stopped.
var ErrNotStarted = errors.New("notify receiver: not started")

// NewReceiver assembles a Receiver but does NOT bind the listener. Call
// Start to bind and begin serving. This separation lets the caller
// distinguish config errors (returned from NewReceiver) from runtime network
// errors (returned from Start).
//
// Returns (nil, nil) when cfg.ListenAddr is empty.
func NewReceiver(cfg Config) (*Receiver, error) {
	if cfg.ListenAddr == "" {
		return nil, nil
	}

	// NewCCMServerConfig builds a gotls.Config with CCM-8 as the primary
	// cipher, GCM as fallback, RequireAnyClientCert, and the
	// HardwareModuleName-SAN-tolerant verify hook. This matches the posture
	// of the production sep2tls CCM server configuration.
	tlsCfg, err := sepTLS.NewCCMServerConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("notify receiver: build TLS config: %w", err)
	}

	d := cfg.Dispatcher
	if d == nil {
		d = Noop
	}

	return &Receiver{
		tlsCfg:     tlsCfg,
		listenAddr: cfg.ListenAddr,
		dispatcher: d,
		logger:     cfg.Logger,
	}, nil
}

// Start binds the listener and begins serving in a goroutine. The bound
// address is available via Addr immediately after Start returns nil.
//
// A second call returns ErrAlreadyStarted. Restart-after-Stop is not
// supported: the http.Server is single-use by design.
func (r *Receiver) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.srv != nil {
		return ErrAlreadyStarted
	}

	tcpL, err := net.Listen("tcp", r.listenAddr)
	if err != nil {
		return fmt.Errorf("notify receiver: listen %s: %w", r.listenAddr, err)
	}
	mux := http.NewServeMux()
	mux.Handle("/notify", r.handler())

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
		// Store the *gotls.Conn in the per-connection context so the handler
		// can read PeerCertificates after the TLS handshake completes.
		// http.Server does not recognise gotls.Conn as *crypto/tls.Conn and
		// therefore does not populate req.TLS itself.
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			if tc, ok := c.(*gotls.Conn); ok {
				return context.WithValue(ctx, gotlsConnKey{}, tc)
			}
			return ctx
		},
	}
	// net/http does not run or log the handshake for a *gotls.Conn, so the
	// wrapper logs refused handshakes where net/http logs its own errors.
	tlsL := sepTLS.WrapCCMListener(gotls.NewListener(tcpL, r.tlsCfg), srv.ErrorLog)

	doneCh := make(chan error, 1)
	r.tlsL = tlsL
	r.srv = srv
	r.doneCh = doneCh

	// Capture doneCh in the goroutine closure so Stop can clear r.doneCh
	// without racing with the goroutine's send.
	go func() {
		err := srv.Serve(tlsL)
		if errors.Is(err, http.ErrServerClosed) {
			doneCh <- nil
			return
		}
		doneCh <- err
	}()

	return nil
}

// Addr returns the bound listener address, including any random-port
// resolution when the configured address used :0. Returns ErrNotStarted
// when Start has not been called (or returned an error).
func (r *Receiver) Addr() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tlsL == nil {
		return "", ErrNotStarted
	}
	return r.tlsL.Addr().String(), nil
}

// Stop initiates a graceful shutdown and waits for the serve goroutine to
// exit. The supplied ctx is used as the parent for an internal shutdown
// context capped at shutdownTimeout (5 s); if the caller's ctx expires
// earlier that shorter deadline applies instead. An immediate Close is
// forced when graceful shutdown does not complete within the cap.
//
// Stop is safe to call multiple times: the first call drains the serve
// goroutine; subsequent calls return ErrNotStarted.
func (r *Receiver) Stop(ctx context.Context) error {
	r.mu.Lock()
	srv := r.srv
	doneCh := r.doneCh
	r.srv = nil
	r.doneCh = nil
	r.tlsL = nil
	r.mu.Unlock()

	if srv == nil {
		return ErrNotStarted
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	defer cancel()

	// Shutdown closes the underlying listener as part of its work.
	if err := srv.Shutdown(shutdownCtx); err != nil {
		// srv.Close is safe after a partial Shutdown: it force-closes any
		// lingering connections so Stop does not block indefinitely.
		_ = srv.Close()
	}

	// Wait for the Serve goroutine to drain. doneCh receives exactly once.
	return <-doneCh
}

// handler returns the http.HandlerFunc bound to POST /notify.
//
// Status code policy (IEEE 2030.5 section 10.13):
//   - 405 Method Not Allowed for any non-POST method.
//   - 415 Unsupported Media Type when the base media type is not application/sep+xml.
//   - 400 Bad Request on read failure, body-too-large, empty body, malformed
//     XML, or XML that does not decode into a Notification.
//   - 500 Internal Server Error when the connection context yields no verified
//     peer certificate (fail-closed guard enforcing the Dispatcher contract),
//     or when the Dispatcher panics.
//   - 204 No Content on a well-formed, accepted Notification.
func (r *Receiver) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// IEEE 2030.5 section 10 / CSIP section 6.6 mandates application/sep+xml. Use
		// mime.ParseMediaType to extract the base type so that valid
		// variants such as "application/sep+xml; charset=utf-8" or
		// uppercased values are accepted.
		ct := req.Header.Get("Content-Type")
		baseType, _, parseErr := mime.ParseMediaType(ct)
		if parseErr != nil || baseType != contentTypeSEPXML {
			http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
			return
		}

		body, err := io.ReadAll(io.LimitReader(req.Body, maxBodyBytes+1))
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		if len(body) > maxBodyBytes {
			http.Error(w, "body too large", http.StatusBadRequest)
			return
		}
		if len(body) == 0 {
			http.Error(w, "empty body", http.StatusBadRequest)
			return
		}

		var n sep2.Notification
		if err := xml.Unmarshal(body, &n); err != nil {
			http.Error(w, "malformed XML", http.StatusBadRequest)
			return
		}
		// A Notification with all zero/empty fields almost certainly means
		// the XML decoded into the wrong type. In practice every CORE-018
		// Notification carries at least one of these fields.
		if n.SubscribedResource == "" && n.NewResourceURI == "" && n.Status == 0 {
			http.Error(w, "notification missing subscribedResource and newResourceURI", http.StatusBadRequest)
			return
		}

		// Extract the verified peer leaf certificate for the dispatcher.
		// gotls.Conn is not *crypto/tls.Conn so req.TLS is nil; retrieve
		// the post-handshake state via the conn reference stored in
		// ConnContext above. Chain validation already ran in
		// VerifyPeerCertificate; PeerCertificates[0] is trust-verified.
		// RequireAnyClientCert guarantees the slice is non-empty after a
		// successful handshake.
		var peerCert *x509.Certificate
		if tc, ok := req.Context().Value(gotlsConnKey{}).(*gotls.Conn); ok {
			if cs := tc.ConnectionState(); len(cs.PeerCertificates) > 0 {
				peerCert = cs.PeerCertificates[0]
			}
		}

		// Fail closed when the peer cert is not available. In production the
		// ConnContext + RequireAnyClientCert guarantee a non-nil cert; if
		// either is absent (a plain-HTTP connection, a test helper, or a
		// future refactor) the handler returns 500 before invoking the
		// dispatcher. This enforces the peerCert-is-never-nil contract in
		// the Dispatcher godoc rather than relying on documentation alone.
		if peerCert == nil {
			if r.logger != nil {
				r.logger.ErrorContext(req.Context(), "notify: no verified peer certificate in connection context")
			}
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		// Invoke the dispatcher inside a recover wrapper so a panicking
		// consumer cannot crash the receiver process. A panic returns 500.
		var recoverVal any
		func() {
			defer func() { recoverVal = recover() }()
			r.dispatcher(req.Context(), peerCert, n)
		}()
		if recoverVal != nil {
			if r.logger != nil {
				r.logger.ErrorContext(req.Context(), "dispatcher panicked",
					"recover", fmt.Sprint(recoverVal))
			}
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
