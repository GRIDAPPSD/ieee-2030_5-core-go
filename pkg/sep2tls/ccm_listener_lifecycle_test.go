package sep2tls_test

import (
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2cert"
	sepTLS "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls"
	gotls "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls/gotls"
)

const handshakeFrame = "sep2tls.(*ccmLoggingListener).handshake"

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
	wrapped := sepTLS.WrapCCMListener(gotls.NewListener(tcpListener, cfg), errorLog.Printf)

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
	wrapped := sepTLS.WrapCCMListener(tcpListener, log.New(logBuf, "", 0).Printf)

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
