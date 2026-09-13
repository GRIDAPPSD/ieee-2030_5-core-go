package sep2tls_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2cert"
	sepTLS "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls"
	gotls "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls/gotls"
)

const (
	acceptLoopFrame = "sep2tls.(*ccmLoggingListener).acceptLoop"
	handshakeFrame  = "sep2tls.(*ccmLoggingListener).handshake"
	// A goroutine that has not run yet shows only its creator in a stack
	// dump, so the constructor frame is what catches a just-started loop.
	constructorFrame = "sep2tls.WrapCCMListener"
)

// goroutinesIn counts running goroutines whose stack contains frame.
func goroutinesIn(frame string) int {
	buf := make([]byte, 1<<16)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	count := 0
	for _, g := range strings.Split(string(buf), "\n\n") {
		if strings.Contains(g, frame) {
			count++
		}
	}
	return count
}

// waitGoroutinesGone fails t unless no goroutine runs any of frames within
// timeout.
func waitGoroutinesGone(t *testing.T, timeout time.Duration, frames ...string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		left := 0
		for _, f := range frames {
			left += goroutinesIn(f)
		}
		if left == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d goroutines still in %v after %s", left, frames, timeout)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// within runs fn and fails t if it has not returned after timeout.
func within(t *testing.T, timeout time.Duration, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("%s did not return within %s", what, timeout)
	}
}

func ccmClientConfig(t *testing.T, files ccmTestFiles) *gotls.Config {
	t.Helper()
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(files.caPEM) {
		t.Fatal("failed to parse CA cert into pool")
	}
	deviceCert, err := gotls.X509KeyPair(files.devicePEM, files.deviceKey)
	if err != nil {
		t.Fatalf("gotls.X509KeyPair (device cert): %v", err)
	}
	return &gotls.Config{
		RootCAs:          caPool,
		Certificates:     []gotls.Certificate{deviceCert},
		MinVersion:       gotls.VersionTLS12,
		MaxVersion:       gotls.VersionTLS12,
		CipherSuites:     []uint16{gotls.TLS_ECDHE_ECDSA_WITH_AES_128_CCM_8},
		CurvePreferences: []gotls.CurveID{gotls.CurveP256},
	}
}

// ccmHTTPClient opens a new CCM-8 connection for every request, so each
// request exercises one full handshake through the listener under test.
func ccmHTTPClient(t *testing.T, files ccmTestFiles) *http.Client {
	t.Helper()
	cfg := ccmClientConfig(t, files)
	return &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
			DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return (&gotls.Dialer{Config: cfg}).DialContext(ctx, network, addr)
			},
		},
	}
}

type temporaryAcceptError struct{}

func (temporaryAcceptError) Error() string   { return "injected temporary accept error" }
func (temporaryAcceptError) Timeout() bool   { return false }
func (temporaryAcceptError) Temporary() bool { return true }

// flakyListener fails its first failures Accept calls with a temporary error,
// as accept(2) does under file descriptor exhaustion.
type flakyListener struct {
	net.Listener
	failures atomic.Int32
}

func (l *flakyListener) Accept() (net.Conn, error) {
	if l.failures.Add(-1) >= 0 {
		return nil, temporaryAcceptError{}
	}
	return l.Listener.Accept()
}

// signalListener reports each accepted connection on accepted, so a test can
// act after the wrapper has taken a connection but before it is handed on.
type signalListener struct {
	net.Listener
	accepted chan struct{}
}

func (l *signalListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err == nil {
		l.accepted <- struct{}{}
	}
	return c, err
}

func waitAccepted(t *testing.T, accepted <-chan struct{}) {
	t.Helper()
	select {
	case <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("listener did not accept the connection within 2s")
	}
}

// TestCCMListenerServesSuccessfulHandshakes proves a handshaken connection
// reaches the handler unchanged: still a *gotls.Conn carrying the verified
// peer certificate, not delayed by a peer that never speaks, and not logged.
func TestCCMListenerServesSuccessfulHandshakes(t *testing.T) {
	files := newCCMTestFiles(t)
	cfg := newCCMServerConfig(t, files)
	deviceCert, err := sep2cert.ParseCertificatePEM(files.devicePEM)
	if err != nil {
		t.Fatalf("parse device cert: %v", err)
	}

	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	logBuf := newSyncLogBuf()
	errorLog := log.New(logBuf, "", 0)
	wrapped := sepTLS.WrapCCMListener(gotls.NewListener(tcpListener, cfg), errorLog)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "no gotls connection state", http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprintf(w, "%04x %04x %s", r.TLS.Version, r.TLS.CipherSuite, r.TLS.PeerCertificates[0].SerialNumber)
	})
	srv := &http.Server{Handler: sepTLS.CCMIdentityMiddleware(handler), ErrorLog: errorLog}
	sepTLS.SetupCCMServer(srv)
	go func() { _ = srv.Serve(wrapped) }()
	defer func() { _ = srv.Close() }()

	// A peer holding a handshake open must not delay the connections behind it.
	silent, err := net.Dial("tcp", tcpListener.Addr().String())
	if err != nil {
		t.Fatalf("dial silent peer: %v", err)
	}
	defer func() { _ = silent.Close() }()

	want := fmt.Sprintf("%04x %04x %s", gotls.VersionTLS12, gotls.TLS_ECDHE_ECDSA_WITH_AES_128_CCM_8, deviceCert.SerialNumber)
	client := ccmHTTPClient(t, files)
	for i := range 3 {
		resp, err := client.Get("https://" + tcpListener.Addr().String() + "/")
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatalf("request %d: read body: %v", i, err)
		}
		if resp.StatusCode != http.StatusOK || string(body) != want {
			t.Fatalf("request %d: status %d body %q, want 200 %q", i, resp.StatusCode, body, want)
		}
	}
	if got := logBuf.String(); got != "" {
		t.Errorf("server log = %q, want nothing for successful handshakes", got)
	}
}

// TestCCMListenerPassesNonTLSConnectionsThrough proves a connection that is
// not a *gotls.Conn is served as it was accepted.
func TestCCMListenerPassesNonTLSConnectionsThrough(t *testing.T) {
	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	logBuf := newSyncLogBuf()
	wrapped := sepTLS.WrapCCMListener(tcpListener, log.New(logBuf, "", 0))

	type connTypeKey struct{}
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, r.Context().Value(connTypeKey{}).(string))
		}),
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			return context.WithValue(ctx, connTypeKey{}, fmt.Sprintf("%T", c))
		},
	}
	go func() { _ = srv.Serve(wrapped) }()
	defer func() { _ = srv.Close() }()

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + tcpListener.Addr().String() + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != http.StatusOK || string(body) != "*net.TCPConn" {
		t.Errorf("status %d body %q, want 200 %q", resp.StatusCode, body, "*net.TCPConn")
	}
}

// TestCCMListenerRetriesTemporaryAcceptError proves a temporary Accept error
// reaches net/http, which retries, and the listener keeps accepting and still
// shuts down within a bound afterwards.
func TestCCMListenerRetriesTemporaryAcceptError(t *testing.T) {
	files := newCCMTestFiles(t)
	cfg := newCCMServerConfig(t, files)

	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	flaky := &flakyListener{Listener: gotls.NewListener(tcpListener, cfg)}
	flaky.failures.Store(2)
	logBuf := newSyncLogBuf()
	errorLog := log.New(logBuf, "", 0)
	wrapped := sepTLS.WrapCCMListener(flaky, errorLog)

	srv := &http.Server{Handler: http.NewServeMux(), ErrorLog: errorLog}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(wrapped) }()

	resp, err := ccmHTTPClient(t, files).Get("https://" + tcpListener.Addr().String() + "/")
	if err != nil {
		t.Errorf("request after temporary Accept errors: %v", err)
	} else {
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d, want 404 from the empty mux", resp.StatusCode)
		}
	}
	if got := logBuf.String(); !strings.Contains(got, "Accept error: injected temporary accept error") {
		t.Errorf("server log = %q, want net/http's Accept retry line", got)
	}

	within(t, 3*time.Second, "Shutdown", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	within(t, 2*time.Second, "Serve", func() {
		if err := <-served; !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("Serve returned %v, want http.ErrServerClosed", err)
		}
	})
	waitGoroutinesGone(t, time.Second, acceptLoopFrame, handshakeFrame, constructorFrame)
}

// TestCCMListenerCloseReleasesGoroutines proves Close leaves no accept or
// handshake goroutine and no open connection behind, and that every Accept
// after Close returns an error instead of blocking.
func TestCCMListenerCloseReleasesGoroutines(t *testing.T) {
	tests := []struct {
		name string
		// beforeClose runs against the listener's address and returns a
		// check to run after Close.
		beforeClose func(t *testing.T, addr string, files ccmTestFiles, accepted <-chan struct{}) (afterClose func(t *testing.T))
	}{
		{
			name: "no connection and no Accept caller",
			beforeClose: func(*testing.T, string, ccmTestFiles, <-chan struct{}) func(*testing.T) {
				return func(*testing.T) {}
			},
		},
		{
			name: "handshake completed but never accepted",
			beforeClose: func(t *testing.T, addr string, files ccmTestFiles, accepted <-chan struct{}) func(*testing.T) {
				conn, err := gotls.Dial("tcp", addr, ccmClientConfig(t, files))
				if err != nil {
					t.Fatalf("dial: %v", err)
				}
				waitAccepted(t, accepted)
				return func(t *testing.T) {
					defer func() { _ = conn.Close() }()
					assertClosedByServer(t, conn)
				}
			},
		},
		{
			// Close() waits on l.wg before returning, so the server side of
			// this connection has already been cancelled and torn down by
			// the time this closure runs: a handshake begun here cannot
			// complete. Named and asserted for that outcome, not the
			// aspirational "finishes" this subtest was previously titled.
			name: "handshake begun only after Close never completes",
			beforeClose: func(t *testing.T, addr string, files ccmTestFiles, accepted <-chan struct{}) func(*testing.T) {
				raw, err := net.Dial("tcp", addr)
				if err != nil {
					t.Fatalf("dial: %v", err)
				}
				waitAccepted(t, accepted)
				return func(t *testing.T) {
					cfg := ccmClientConfig(t, files)
					cfg.ServerName = "127.0.0.1"
					conn := gotls.Client(raw, cfg)
					defer func() { _ = conn.Close() }()
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					defer cancel()
					if err := conn.HandshakeContext(ctx); err == nil {
						t.Fatal("client handshake succeeded, want the already-closed server side to refuse it")
					}
					assertClosedByServer(t, conn)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := newCCMTestFiles(t)
			cfg := newCCMServerConfig(t, files)
			tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("Listen: %v", err)
			}
			signal := &signalListener{Listener: gotls.NewListener(tcpListener, cfg), accepted: make(chan struct{}, 1)}
			wrapped := sepTLS.WrapCCMListener(signal, log.New(io.Discard, "", 0))

			afterClose := tt.beforeClose(t, tcpListener.Addr().String(), files, signal.accepted)
			within(t, 2*time.Second, "Close", func() { _ = wrapped.Close() })
			afterClose(t)
			waitGoroutinesGone(t, time.Second, acceptLoopFrame, handshakeFrame, constructorFrame)

			for i := range 2 {
				within(t, time.Second, fmt.Sprintf("Accept %d after Close", i+1), func() {
					if c, err := wrapped.Accept(); err == nil {
						_ = c.Close()
						t.Errorf("Accept %d after Close returned a connection, want an error", i+1)
					}
				})
			}
		})
	}
}

// assertClosedByServer fails t unless the server side of conn has been closed,
// which a read observes as an error other than its own deadline expiring.
func assertClosedByServer(t *testing.T, conn net.Conn) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err := conn.Read(make([]byte, 1))
	var ne net.Error
	if err == nil || (errors.As(err, &ne) && ne.Timeout()) {
		t.Errorf("read after Close = %v, want the server to have closed the connection", err)
	}
}

// TestCCMServerStopsWithHandshakeInFlight proves Shutdown and Close return
// promptly while a peer holds a handshake open, and take its goroutine down.
func TestCCMServerStopsWithHandshakeInFlight(t *testing.T) {
	tests := []struct {
		name string
		stop func(srv *http.Server) error
	}{
		{name: "Shutdown", stop: func(srv *http.Server) error {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			return srv.Shutdown(ctx)
		}},
		{name: "Close", stop: func(srv *http.Server) error { return srv.Close() }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := newCCMTestFiles(t)
			cfg := newCCMServerConfig(t, files)
			tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("Listen: %v", err)
			}
			signal := &signalListener{Listener: gotls.NewListener(tcpListener, cfg), accepted: make(chan struct{}, 1)}
			wrapped := sepTLS.WrapCCMListener(signal, log.New(io.Discard, "", 0))
			srv := &http.Server{Handler: http.NewServeMux()}
			served := make(chan error, 1)
			go func() { served <- srv.Serve(wrapped) }()

			silent, err := net.Dial("tcp", tcpListener.Addr().String())
			if err != nil {
				t.Fatalf("dial silent peer: %v", err)
			}
			defer func() { _ = silent.Close() }()
			waitAccepted(t, signal.accepted)

			within(t, 3*time.Second, tt.name, func() {
				if err := tt.stop(srv); err != nil {
					t.Errorf("%s: %v", tt.name, err)
				}
			})
			within(t, 2*time.Second, "Serve", func() { <-served })
			waitGoroutinesGone(t, time.Second, acceptLoopFrame, handshakeFrame, constructorFrame)
		})
	}
}

// TestWrapCCMListenerNilLoggerUsesStandardLogger proves a nil logger does not
// panic on a refused handshake and the refusal reaches the standard logger,
// as it does for net/http with a nil ErrorLog.
func TestWrapCCMListenerNilLoggerUsesStandardLogger(t *testing.T) {
	files := newCCMTestFiles(t)
	cfg := newCCMServerConfig(t, files)

	logBuf := newSyncLogBuf()
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(logBuf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	}()

	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	wrapped := sepTLS.WrapCCMListener(gotls.NewListener(tcpListener, cfg), nil)
	srv := &http.Server{Handler: http.NewServeMux()}
	go func() { _ = srv.Serve(wrapped) }()
	defer within(t, 3*time.Second, "Close", func() { _ = srv.Close() })

	if conn, err := dialTLS13Stdlib(t, tcpListener.Addr().String(), files, true); err == nil {
		_ = conn.Close()
		t.Fatal("expected a TLS 1.3-only client to be refused, the dial succeeded")
	}
	line := waitForLogLine(t, logBuf, 2*time.Second)
	if !strings.Contains(line, "TLS handshake error") {
		t.Errorf("standard log = %q, want it to name a TLS handshake error", line)
	}
}

// TestCCMListenerClosesSilentPeerAfterHandshakeBound proves a peer that opens
// the connection and never sends a ClientHello is closed once the handshake
// bound elapses, rather than held indefinitely. Not parallel: it shrinks the
// package-level handshake bound (export_test.go) for its duration.
func TestCCMListenerClosesSilentPeerAfterHandshakeBound(t *testing.T) {
	restore := sepTLS.SetCCMHandshakeTimeoutForTest(150 * time.Millisecond)
	defer restore()

	files := newCCMTestFiles(t)
	cfg := newCCMServerConfig(t, files)
	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	wrapped := sepTLS.WrapCCMListener(gotls.NewListener(tcpListener, cfg), log.New(io.Discard, "", 0))
	defer func() { _ = wrapped.Close() }()

	silent, err := net.Dial("tcp", tcpListener.Addr().String())
	if err != nil {
		t.Fatalf("dial silent peer: %v", err)
	}
	defer func() { _ = silent.Close() }()

	// assertClosedByServer's 2s window is well over the shrunk 150ms bound, so
	// a mutant that removes the bound or lengthens it stays open past the
	// window and fails here instead of passing silently.
	assertClosedByServer(t, silent)
}

// TestCCMListenerClosesConnectionAfterFailedHandshake proves the wrapper
// closes the accepted connection itself when the TLS handshake fails for a
// protocol reason (not just the timeout bound above), so the peer observes
// the close rather than a connection left open behind a TLS-level refusal.
func TestCCMListenerClosesConnectionAfterFailedHandshake(t *testing.T) {
	files := newCCMTestFiles(t)
	cfg := newCCMServerConfig(t, files)
	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	wrapped := sepTLS.WrapCCMListener(gotls.NewListener(tcpListener, cfg), log.New(io.Discard, "", 0))
	srv := &http.Server{Handler: http.NewServeMux()}
	go func() { _ = srv.Serve(wrapped) }()
	defer func() { _ = srv.Close() }()

	raw, err := net.Dial("tcp", tcpListener.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = raw.Close() }()

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(files.caPEM) {
		t.Fatal("failed to parse CA cert into pool")
	}
	client := tls.Client(raw, &tls.Config{
		RootCAs:          caPool,
		ServerName:       "127.0.0.1",
		MinVersion:       tls.VersionTLS13,
		MaxVersion:       tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{tls.CurveP256},
	})
	if err := client.Handshake(); err == nil {
		t.Fatal("expected a TLS 1.3-only client to be refused, the handshake succeeded")
	}

	// raw, not client: proves the underlying connection was closed, not just
	// that the tls.Conn abstraction gave up on it.
	assertClosedByServer(t, raw)
}
