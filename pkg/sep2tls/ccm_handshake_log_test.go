package sep2tls_test

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2cert"
	sepTLS "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls"
	gotls "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls/gotls"
)

// capCertSet mints a CA, a server cert, and a device (client) cert, shared
// by TestStdListenerLogsTLS13OnlyRefusal (as a plain stdlib config, unrelated
// to sep2tls's own CCM-only constructors) and TestCCMListenerLogsTLS13OnlyRefusal.
// Formerly lived in the now-removed tls12_cap_test.go, whose stdlib
// GCM-serving tests were subsumed by this file and ccm_tls12_cap_test.go
// once core stopped offering GCM under any name.
type capCertSet struct {
	caPEM     []byte
	serverPEM []byte
	serverKey []byte
	devicePEM []byte
	deviceKey []byte
}

func newCapCertSet(t *testing.T) capCertSet {
	t.Helper()

	caCertPEM, caKeyPEM, err := sep2cert.GenerateCA(sep2cert.CAOptions{
		CommonName: "TLS12 Cap Test CA",
		ValidYears: 1,
	})
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	caCert, caKey := parseCACert(t, caCertPEM, caKeyPEM)

	serverCertPEM, serverKeyPEM, err := sep2cert.GenerateServerCert(caCert, caKey, sep2cert.ServerCertOptions{
		Hosts:      []string{"127.0.0.1", "localhost"},
		CommonName: "TLS12 Cap Test Server",
		ValidYears: 1,
	})
	if err != nil {
		t.Fatalf("GenerateServerCert: %v", err)
	}

	deviceCertPEM, deviceKeyPEM, err := sep2cert.GenerateDeviceCert(caCert, caKey, sep2cert.DeviceCertOptions{
		DeviceType:  sep2cert.DeviceTypeGeneric,
		HWSerialNum: "TLS12-CAP-TEST-001",
	})
	if err != nil {
		t.Fatalf("GenerateDeviceCert: %v", err)
	}

	return capCertSet{
		caPEM:     caCertPEM,
		serverPEM: serverCertPEM,
		serverKey: serverKeyPEM,
		devicePEM: deviceCertPEM,
		deviceKey: deviceKeyPEM,
	}
}

// syncLogBuf is an io.Writer safe for concurrent use by a *log.Logger and a
// test goroutine, that signals on seen after every Write so a test can wait
// for a log line deterministically instead of polling or sleeping.
type syncLogBuf struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	seen chan struct{}
}

func newSyncLogBuf() *syncLogBuf {
	return &syncLogBuf{seen: make(chan struct{}, 1)}
}

func (b *syncLogBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	n, err := b.buf.Write(p)
	b.mu.Unlock()
	select {
	case b.seen <- struct{}{}:
	default:
	}
	return n, err
}

func (b *syncLogBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// waitForLogLine blocks until logBuf receives a write or timeout elapses,
// then returns the buffer's contents at that point.
func waitForLogLine(t *testing.T, logBuf *syncLogBuf, timeout time.Duration) string {
	t.Helper()
	select {
	case <-logBuf.seen:
	case <-time.After(timeout):
		t.Fatalf("no server log line within %s", timeout)
	}
	return logBuf.String()
}

// TestStdListenerLogsTLS13OnlyRefusal is the control for
// TestCCMListenerLogsTLS13OnlyRefusal: net/http's own *tls.Conn handling logs
// a TLS 1.3-only client's refusal on the standard-library listener. Core no
// longer exports a *tls.Config constructor (crypto/tls cannot carry CCM-8),
// so this control builds its own plain stdlib server config: what is under
// test here is net/http's baseline logging behavior for *tls.Conn, not any
// sep2tls constructor.
func TestStdListenerLogsTLS13OnlyRefusal(t *testing.T) {
	certs := newCapCertSet(t)
	serverCert, err := tls.X509KeyPair(certs.serverPEM, certs.serverKey)
	if err != nil {
		t.Fatalf("X509KeyPair (server cert): %v", err)
	}
	serverTLSCfg := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		MinVersion:   tls.VersionTLS12,
		MaxVersion:   tls.VersionTLS12,
	}

	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	logBuf := newSyncLogBuf()
	srv := &http.Server{Handler: http.NewServeMux(), ErrorLog: log.New(logBuf, "", 0)}
	go func() { _ = srv.Serve(tls.NewListener(tcpListener, serverTLSCfg)) }()
	defer func() { _ = srv.Close() }()

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(certs.caPEM) {
		t.Fatal("failed to parse CA cert into pool")
	}
	deviceCert, err := tls.X509KeyPair(certs.devicePEM, certs.deviceKey)
	if err != nil {
		t.Fatalf("X509KeyPair (device cert): %v", err)
	}
	clientCfg := &tls.Config{
		RootCAs:          caPool,
		Certificates:     []tls.Certificate{deviceCert},
		MinVersion:       tls.VersionTLS13,
		MaxVersion:       tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{tls.CurveP256},
	}

	conn, dialErr := tls.Dial("tcp", tcpListener.Addr().String(), clientCfg)
	if dialErr == nil {
		_ = conn.Close()
		t.Fatal("expected a TLS 1.3-only client to be refused, the dial succeeded")
	}

	line := waitForLogLine(t, logBuf, 2*time.Second)
	if !strings.Contains(line, "TLS handshake error") {
		t.Errorf("server log = %q, want it to name a TLS handshake error", line)
	}
}

// TestCCMListenerLogsTLS13OnlyRefusal proves a TLS 1.3-only client refused
// on the gotls (CCM) listener produces a server log entry, the same as the
// standard-library path (TestStdListenerLogsTLS13OnlyRefusal above). Without
// sepTLS.WrapCCMListener, net/http never performs or logs the handshake
// itself for a *gotls.Conn (it is not a *tls.Conn), so the refusal reaches
// no log line at all.
func TestCCMListenerLogsTLS13OnlyRefusal(t *testing.T) {
	files := newCCMTestFiles(t)
	cfg := newCCMServerConfig(t, files)

	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	logBuf := newSyncLogBuf()
	wrapped := sepTLS.WrapCCMListener(gotls.NewListener(tcpListener, cfg), log.New(logBuf, "", 0))
	srv := &http.Server{Handler: http.NewServeMux()}
	go func() { _ = srv.Serve(wrapped) }()
	defer func() { _ = srv.Close() }()

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(files.caPEM) {
		t.Fatal("failed to parse CA cert into pool")
	}
	deviceCert, err := tls.X509KeyPair(files.devicePEM, files.deviceKey)
	if err != nil {
		t.Fatalf("X509KeyPair (device cert): %v", err)
	}
	clientCfg := &tls.Config{
		RootCAs:          caPool,
		Certificates:     []tls.Certificate{deviceCert},
		MinVersion:       tls.VersionTLS13,
		MaxVersion:       tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{tls.CurveP256},
	}

	conn, dialErr := tls.Dial("tcp", tcpListener.Addr().String(), clientCfg)
	if dialErr == nil {
		_ = conn.Close()
		t.Fatal("expected a TLS 1.3-only client to be refused, the dial succeeded")
	}

	waitForLogLine(t, logBuf, 2*time.Second)
	// The handshake goroutine exits only after it has logged, so the count
	// is final once it is gone.
	waitGoroutinesGone(t, 2*time.Second, handshakeFrame)
	logged := logBuf.String()
	if n := strings.Count(logged, "TLS handshake error"); n != 1 {
		t.Errorf("server log = %q, want exactly one TLS handshake error line, got %d", logged, n)
	}
	if !strings.Contains(logged, "unsupported versions") {
		t.Errorf("server log = %q, want the refusal cause (unsupported versions)", logged)
	}
}
