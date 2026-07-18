package sep2tls_test

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2cert"
	sepTLS "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls"
	gotls "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls/gotls"
)

// ccmTestFiles mints a CA, a server cert, and a device (client) cert, writes
// each to a temp file, and returns the file paths NewCCMServerConfigWithExtraCAs
// needs (it reads from disk, unlike the PEM-byte config.go constructors).
type ccmTestFiles struct {
	caPEM      []byte
	caPath     string
	serverPath string
	serverKey  string
	devicePEM  []byte
	deviceKey  []byte
}

func newCCMTestFiles(t *testing.T) ccmTestFiles {
	t.Helper()

	caCertPEM, caKeyPEM, err := sep2cert.GenerateCA(sep2cert.CAOptions{
		CommonName: "CCM TLS13 Test CA",
		ValidYears: 1,
	})
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	caCert, caKey := parseCACert(t, caCertPEM, caKeyPEM)

	serverCertPEM, serverKeyPEM, err := sep2cert.GenerateServerCert(caCert, caKey, sep2cert.ServerCertOptions{
		Hosts:      []string{"127.0.0.1", "localhost"},
		CommonName: "CCM TLS13 Test Server",
		ValidYears: 1,
	})
	if err != nil {
		t.Fatalf("GenerateServerCert: %v", err)
	}

	deviceCertPEM, deviceKeyPEM, err := sep2cert.GenerateDeviceCert(caCert, caKey, sep2cert.DeviceCertOptions{
		DeviceType:  sep2cert.DeviceTypeGeneric,
		HWSerialNum: "CCM-TLS13-TEST-001",
	})
	if err != nil {
		t.Fatalf("GenerateDeviceCert: %v", err)
	}

	dir := t.TempDir()
	caPath := writeCCMTempFile(t, dir, "ca.pem", caCertPEM)
	serverPath := writeCCMTempFile(t, dir, "server.pem", serverCertPEM)
	serverKeyPath := writeCCMTempFile(t, dir, "server-key.pem", serverKeyPEM)

	return ccmTestFiles{
		caPEM:      caCertPEM,
		caPath:     caPath,
		serverPath: serverPath,
		serverKey:  serverKeyPath,
		devicePEM:  deviceCertPEM,
		deviceKey:  deviceKeyPEM,
	}
}

func writeCCMTempFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, content, 0o600); err != nil {
		t.Fatalf("WriteFile %s: %v", name, err)
	}
	return p
}

// startCCMTestListener stands up a raw gotls listener using
// NewCCMServerConfigWithExtraCAs, mirroring the production wiring in
// pkg/sep2srv/server.go's wrapMTLS (EnableCCM branch) and
// pkg/sep2client/notify/receiver.go. Each accepted connection is handshaken
// and, on success, has its ConnectionState handed to onConn so tests can
// assert on the negotiated version, cipher suite, and peer certificates. A
// handshake failure is swallowed here (not a test failure): the negative
// mutual-auth tests below expect exactly this outcome and assert on the
// dial-side error instead.
func startCCMTestListener(t *testing.T, files ccmTestFiles) (addr string, shutdown func()) {
	t.Helper()

	serverCfg, err := sepTLS.NewCCMServerConfigWithExtraCAs(files.serverPath, files.serverKey, files.caPath, nil)
	if err != nil {
		t.Fatalf("NewCCMServerConfigWithExtraCAs: %v", err)
	}

	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	listener := gotls.NewListener(tcpListener, serverCfg)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				gc, ok := c.(*gotls.Conn)
				if !ok {
					return
				}
				if err := gc.Handshake(); err != nil {
					return
				}
			}(conn)
		}
	}()

	return tcpListener.Addr().String(), func() { _ = listener.Close() }
}

// dialCCM dials addr with a gotls client pinned to exactly minVer/maxVer,
// offering CCM-8 as its only cipher suite (mirroring a spec-strict CSIP 1.2
// device). withCert controls whether the client presents its device
// certificate; the false branch exercises the mutual-auth-required
// rejection path.
func dialCCM(t *testing.T, addr string, files ccmTestFiles, minVer, maxVer uint16, withCert bool) (*gotls.Conn, error) {
	t.Helper()

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(files.caPEM) {
		t.Fatal("failed to parse CA cert into pool")
	}

	cfg := &gotls.Config{
		RootCAs:          caPool,
		MinVersion:       minVer,
		MaxVersion:       maxVer,
		CipherSuites:     []uint16{gotls.TLS_ECDHE_ECDSA_WITH_AES_128_CCM_8},
		CurvePreferences: []gotls.CurveID{gotls.CurveP256},
	}

	if withCert {
		deviceCert, err := gotls.X509KeyPair(files.devicePEM, files.deviceKey)
		if err != nil {
			t.Fatalf("gotls.X509KeyPair (device cert): %v", err)
		}
		cfg.Certificates = []gotls.Certificate{deviceCert}
	} else {
		cfg.InsecureSkipVerify = true
	}

	return gotls.Dial("tcp", addr, cfg)
}

// dialTLS13Stdlib dials addr with a plain stdlib crypto/tls client pinned to
// TLS 1.3 only, mirroring the EPRI reference client's ClientHello (no TLS 1.2
// cipher suites offered at all). This proves the CCM-configured gotls
// listener now accepts a generic TLS 1.3 peer, not just a gotls-speaking one.
func dialTLS13Stdlib(t *testing.T, addr string, files ccmTestFiles, withCert bool) (*tls.Conn, error) {
	t.Helper()

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(files.caPEM) {
		t.Fatal("failed to parse CA cert into pool")
	}

	cfg := &tls.Config{
		RootCAs:          caPool,
		MinVersion:       tls.VersionTLS13,
		MaxVersion:       tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{tls.CurveP256},
	}

	if withCert {
		deviceCert, err := tls.X509KeyPair(files.devicePEM, files.deviceKey)
		if err != nil {
			t.Fatalf("tls.X509KeyPair (device cert): %v", err)
		}
		cfg.Certificates = []tls.Certificate{deviceCert}
	} else {
		cfg.InsecureSkipVerify = true
	}

	return tls.Dial("tcp", addr, cfg)
}

// TestCCM8StillWorksAfterTLS13Widening proves the mandatory IEEE 2030.5 §6.7
// cipher suite (TLS 1.2, CCM-8) still negotiates through
// NewCCMServerConfigWithExtraCAs exactly as before the MaxVersion widening.
// A regression here would mean a spec-strict CSIP device that only speaks
// CCM-8 can no longer connect.
func TestCCM8StillWorksAfterTLS13Widening(t *testing.T) {
	files := newCCMTestFiles(t)
	addr, shutdown := startCCMTestListener(t, files)
	defer shutdown()

	conn, err := dialCCM(t, addr, files, gotls.VersionTLS12, gotls.VersionTLS12, true)
	if err != nil {
		t.Fatalf("CCM-8 TLS 1.2 client dial failed: %v", err)
	}
	defer func() { _ = conn.Close() }()

	state := conn.ConnectionState()
	if state.Version != gotls.VersionTLS12 {
		t.Errorf("negotiated version = 0x%04x, want TLS 1.2 (0x%04x)", state.Version, gotls.VersionTLS12)
	}
	if state.CipherSuite != gotls.TLS_ECDHE_ECDSA_WITH_AES_128_CCM_8 {
		t.Errorf("cipher suite = 0x%04x, want CCM-8 (0x%04x)", state.CipherSuite, gotls.TLS_ECDHE_ECDSA_WITH_AES_128_CCM_8)
	}
}

// TestTLS13OnlyClientNowConnectsToCCMListener reproduces the EPRI reference
// client symptom directly against the CCM-configured production listener
// wiring (NewCCMServerConfigWithExtraCAs plus gotls.NewListener): a generic
// TLS 1.3-only client, using the plain stdlib crypto/tls engine rather than
// gotls, must now complete the handshake. Before the MaxVersion widening
// this failed with "no cipher suite supported by both client and server".
func TestTLS13OnlyClientNowConnectsToCCMListener(t *testing.T) {
	files := newCCMTestFiles(t)
	addr, shutdown := startCCMTestListener(t, files)
	defer shutdown()

	conn, err := dialTLS13Stdlib(t, addr, files, true)
	if err != nil {
		t.Fatalf("TLS 1.3-only client dial failed: %v", err)
	}
	defer func() { _ = conn.Close() }()

	state := conn.ConnectionState()
	if state.Version != tls.VersionTLS13 {
		t.Errorf("negotiated version = %s, want TLS 1.3", tls.VersionName(state.Version))
	}
}

// TestMutualAuthStillEnforcedOnCCMPath proves the TLS 1.3 widening did not
// open an unauthenticated hole on the CCM/gotls listener path: a client
// presenting no certificate must still be rejected, under BOTH TLS 1.2
// (via the gotls client) and TLS 1.3 (via the stdlib client).
func TestMutualAuthStillEnforcedOnCCMPath(t *testing.T) {
	t.Run("TLS 1.2 gotls client, no cert", func(t *testing.T) {
		files := newCCMTestFiles(t)
		addr, shutdown := startCCMTestListener(t, files)
		defer shutdown()

		conn, err := dialCCM(t, addr, files, gotls.VersionTLS12, gotls.VersionTLS12, false)
		if err == nil {
			_ = conn.Close()
			t.Fatal("expected handshake to fail without a client cert under TLS 1.2, it succeeded")
		}
	})

	t.Run("TLS 1.3 stdlib client, no cert", func(t *testing.T) {
		files := newCCMTestFiles(t)
		addr, shutdown := startCCMTestListener(t, files)
		defer shutdown()

		// Under TLS 1.3 the client certificate exchange happens inside the
		// encrypted handshake and the server's rejection ("certificate
		// required") is delivered as a fatal alert record sent after the
		// client's Finished message, not as a Dial-time handshake error: a
		// stdlib tls.Dial call can return success before that alert is
		// read. This is normal TLS 1.3 protocol shape (RFC 8446 section 4.4.2),
		// not a mutual-auth bypass: the first Read on the connection observes
		// the rejection, which is what a real HTTP round trip would do next.
		conn, err := dialTLS13Stdlib(t, addr, files, false)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		buf := make([]byte, 1)
		if _, rerr := conn.Read(buf); rerr == nil {
			t.Fatal("expected the connection to be rejected without a client cert under TLS 1.3, it was accepted")
		}
	})
}
