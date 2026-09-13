package sep2tls_test

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		CommonName: "CCM TLS12 Cap Test CA",
		ValidYears: 1,
	})
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	caCert, caKey := parseCACert(t, caCertPEM, caKeyPEM)

	serverCertPEM, serverKeyPEM, err := sep2cert.GenerateServerCert(caCert, caKey, sep2cert.ServerCertOptions{
		Hosts:      []string{"127.0.0.1", "localhost"},
		CommonName: "CCM TLS12 Cap Test Server",
		ValidYears: 1,
	})
	if err != nil {
		t.Fatalf("GenerateServerCert: %v", err)
	}

	deviceCertPEM, deviceKeyPEM, err := sep2cert.GenerateDeviceCert(caCert, caKey, sep2cert.DeviceCertOptions{
		DeviceType:  sep2cert.DeviceTypeGeneric,
		HWSerialNum: "CCM-TLS12-CAP-TEST-001",
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

// startCCMTestListener stands up a raw gotls listener using cfg, mirroring
// the production wiring in pkg/sep2srv/server.go's wrapMTLS (EnableCCM
// branch) and pkg/sep2client/notify/receiver.go. Each accepted connection is
// handshaken; a handshake error is sent to errs if non-nil, so tests can
// assert on the server-side refusal reason (errs may be nil to swallow it,
// matching the negative mutual-auth tests below, which assert on the
// dial-side error instead).
func startCCMTestListener(t *testing.T, cfg *gotls.Config, errs chan error) (addr string, shutdown func()) {
	t.Helper()

	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	listener := gotls.NewListener(tcpListener, cfg)

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
				if err := gc.Handshake(); err != nil && errs != nil {
					select {
					case errs <- err:
					default:
					}
				}
			}(conn)
		}
	}()

	return tcpListener.Addr().String(), func() { _ = listener.Close() }
}

func newCCMServerConfig(t *testing.T, files ccmTestFiles) *gotls.Config {
	t.Helper()
	cfg, err := sepTLS.NewCCMServerConfigWithExtraCAs(files.serverPath, files.serverKey, files.caPath, nil)
	if err != nil {
		t.Fatalf("NewCCMServerConfigWithExtraCAs: %v", err)
	}
	return cfg
}

// dialCCM dials addr with a gotls client pinned to exactly minVer/maxVer,
// offering suites (mirroring a spec-strict CSIP device when suites is just
// CCM-8). withCert controls whether the client presents its device
// certificate; the false branch exercises the mutual-auth-required
// rejection path.
func dialCCM(t *testing.T, addr string, files ccmTestFiles, minVer, maxVer uint16, suites []uint16, withCert bool) (*gotls.Conn, error) {
	t.Helper()

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(files.caPEM) {
		t.Fatal("failed to parse CA cert into pool")
	}

	cfg := &gotls.Config{
		RootCAs:          caPool,
		MinVersion:       minVer,
		MaxVersion:       maxVer,
		CipherSuites:     suites,
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
// cipher suites offered at all).
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

// TestCCM8StillWorksUnderCap proves the mandatory IEEE 2030.5 clause 6.7
// cipher suite (TLS 1.2, CCM-8) still negotiates through
// NewCCMServerConfigWithExtraCAs under the TLS 1.2 cap. A regression here
// would mean a spec-strict CSIP device that only speaks CCM-8 can no longer
// connect.
func TestCCM8StillWorksUnderCap(t *testing.T) {
	files := newCCMTestFiles(t)
	cfg := newCCMServerConfig(t, files)
	addr, shutdown := startCCMTestListener(t, cfg, nil)
	defer shutdown()

	conn, err := dialCCM(t, addr, files, gotls.VersionTLS12, gotls.VersionTLS12, []uint16{gotls.TLS_ECDHE_ECDSA_WITH_AES_128_CCM_8}, true)
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

// TestCCMCapRefusesTLS13OnlyClient is core-go #125 amended criterion 3.1 for
// the CCM constructor. Replaces TestTLS13OnlyClientNowConnectsToCCMListener,
// withdrawn by the amendment. RED at 406baef: NewCCMServerConfigWithExtraCAs's
// MaxVersion was TLS 1.3 there, so this client completed instead of being
// refused.
func TestCCMCapRefusesTLS13OnlyClient(t *testing.T) {
	files := newCCMTestFiles(t)
	cfg := newCCMServerConfig(t, files)
	errs := make(chan error, 4)
	addr, shutdown := startCCMTestListener(t, cfg, errs)
	defer shutdown()

	conn, err := dialTLS13Stdlib(t, addr, files, true)
	if err == nil {
		_ = conn.Close()
		t.Fatal("expected a TLS 1.3-only client to be refused, the dial succeeded")
	}

	select {
	case serverErr := <-errs:
		if !strings.Contains(serverErr.Error(), "unsupported versions") {
			t.Errorf("server-side handshake error = %q, want it to name the unsupported version", serverErr)
		}
	case <-time.After(2 * time.Second):
		t.Error("expected a server-side handshake error, none was captured within 2s")
	}
}

// TestCCMCapNegotiatesTLS12WithCCM8ForDualVersionClient is core-go #125
// amended criterion 3.2 (CCM half): a client offering CCM-8 at TLS 1.2 and
// also capable of TLS 1.3 negotiates TLS 1.2 with CCM-8. RED at 406baef: the
// uncapped config served TLS 1.3 to this client, since that was the higher
// mutually supported version.
func TestCCMCapNegotiatesTLS12WithCCM8ForDualVersionClient(t *testing.T) {
	files := newCCMTestFiles(t)
	cfg := newCCMServerConfig(t, files)
	addr, shutdown := startCCMTestListener(t, cfg, nil)
	defer shutdown()

	conn, err := dialCCM(t, addr, files, gotls.VersionTLS12, gotls.VersionTLS13, []uint16{gotls.TLS_ECDHE_ECDSA_WITH_AES_128_CCM_8}, true)
	if err != nil {
		t.Fatalf("dual-version CCM-8 client dial failed: %v", err)
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

// TestCCM8PreferredOverGCM is core-go #136 criterion 1: a TLS 1.2 client
// offering both CCM-8 and the ECDHE-ECDSA AES-128-GCM suite negotiates
// CCM-8. RED at 406baef: cipher_suites_ccm.go appended CCM-8 to the end of
// the preference order, so pickCipherSuite reached GCM first.
func TestCCM8PreferredOverGCM(t *testing.T) {
	files := newCCMTestFiles(t)
	cfg := newCCMServerConfig(t, files)
	addr, shutdown := startCCMTestListener(t, cfg, nil)
	defer shutdown()

	suites := []uint16{gotls.TLS_ECDHE_ECDSA_WITH_AES_128_CCM_8, 0xC02B}
	conn, err := dialCCM(t, addr, files, gotls.VersionTLS12, gotls.VersionTLS12, suites, true)
	if err != nil {
		t.Fatalf("dial offering CCM-8 and GCM failed: %v", err)
	}
	defer func() { _ = conn.Close() }()

	state := conn.ConnectionState()
	if state.CipherSuite != gotls.TLS_ECDHE_ECDSA_WITH_AES_128_CCM_8 {
		t.Errorf("cipher suite = 0x%04x, want CCM-8 (0x%04x): IEEE 2030.5-2018 clause 6.7 makes it mandatory", state.CipherSuite, gotls.TLS_ECDHE_ECDSA_WITH_AES_128_CCM_8)
	}
}

// TestGCMOnlyClientStillNegotiatesGCM is core-go #136 criterion 3: a client
// offering only the GCM suite still negotiates it against the CCM
// configuration. Not RED: unaffected by the preference-order fix, since
// order only matters when more than one mutually offered suite exists.
func TestGCMOnlyClientStillNegotiatesGCM(t *testing.T) {
	files := newCCMTestFiles(t)
	cfg := newCCMServerConfig(t, files)
	addr, shutdown := startCCMTestListener(t, cfg, nil)
	defer shutdown()

	conn, err := dialCCM(t, addr, files, gotls.VersionTLS12, gotls.VersionTLS12, []uint16{0xC02B}, true)
	if err != nil {
		t.Fatalf("GCM-only client dial failed: %v", err)
	}
	defer func() { _ = conn.Close() }()

	state := conn.ConnectionState()
	if state.CipherSuite != 0xC02B {
		t.Errorf("cipher suite = 0x%04x, want GCM (0xC02B)", state.CipherSuite)
	}
}

// TestCCMCapNoDowngradeSentinel is core-go #125 amended criterion 3.3 (CCM
// half), matching the security review's C4/C4c pairing: a generic stdlib
// client (default max version, so capable of TLS 1.3) completes on TLS 1.2
// against the capped CCM listener with no RFC 8446 section 4.1.3 downgrade
// sentinel, and the same detection code finds the sentinel when a
// TLS 1.3-capable CCM config downgrades a 1.2-only client (the scratch
// uncapped config is built only to prove the detector fires; core no
// longer ships a TLS 1.3-capable CCM config).
func TestCCMCapNoDowngradeSentinel(t *testing.T) {
	files := newCCMTestFiles(t)

	t.Run("capped config writes no sentinel", func(t *testing.T) {
		cfg := newCCMServerConfig(t, files)
		addr, shutdown := startCCMTestListener(t, cfg, nil)
		defer shutdown()

		version, random := recordAndHandshakeCCM(t, addr, files, tls.VersionTLS12, 0)
		if version != tls.VersionTLS12 {
			t.Fatalf("negotiated version = %s, want TLS 1.2: the sentinel check only means something if the server was actually forced down to 1.2", tls.VersionName(version))
		}
		if tail := random[24:]; string(tail) == string(downgradeCanaryTLS12) {
			t.Errorf("capped CCM config wrote the downgrade sentinel %x, it has nothing to downgrade from", tail)
		}
	})

	t.Run("control: an uncapped CCM config does write the sentinel", func(t *testing.T) {
		cfg := newCCMServerConfig(t, files)
		cfg.MaxVersion = gotls.VersionTLS13 // scratch-only: proves the detector, not shipped
		addr, shutdown := startCCMTestListener(t, cfg, nil)
		defer shutdown()

		// A 1.2-only client forces the downgrade path against the 1.3-capable server.
		version, random := recordAndHandshakeCCM(t, addr, files, tls.VersionTLS12, tls.VersionTLS12)
		if version != tls.VersionTLS12 {
			t.Fatalf("negotiated version = %s, want TLS 1.2", tls.VersionName(version))
		}
		if got := random[24:]; string(got) != string(downgradeCanaryTLS12) {
			t.Fatalf("control did not observe the downgrade sentinel (got %x): the detector cannot prove a negative", got)
		}
	})
}

// recordAndHandshakeCCM is the CCM-listener counterpart of recordAndHandshake
// (tls12_cap_test.go): a plain stdlib client over a recordingConn, so the raw
// ServerHello.random is recoverable after the handshake completes. Returns
// the negotiated version alongside it. maxVer of 0 leaves the client's
// default (highest mutually supported).
func recordAndHandshakeCCM(t *testing.T, addr string, files ccmTestFiles, minVer, maxVer uint16) (uint16, [32]byte) {
	t.Helper()

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(files.caPEM) {
		t.Fatal("failed to parse CA cert into pool")
	}
	deviceCert, err := tls.X509KeyPair(files.devicePEM, files.deviceKey)
	if err != nil {
		t.Fatalf("X509KeyPair (device cert): %v", err)
	}

	rawConn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("net.Dial: %v", err)
	}
	defer func() { _ = rawConn.Close() }()

	rec := &recordingConn{Conn: rawConn, buf: new(bytes.Buffer)}
	tlsConn := tls.Client(rec, &tls.Config{
		RootCAs: caPool,
		// tls.Client (unlike tls.Dial) never derives ServerName from the
		// address, and the cert covers 127.0.0.1 as a SAN.
		ServerName:       "127.0.0.1",
		Certificates:     []tls.Certificate{deviceCert},
		MinVersion:       minVer,
		MaxVersion:       maxVer,
		CurvePreferences: []tls.CurveID{tls.CurveP256},
	})
	defer func() { _ = tlsConn.Close() }()

	if err := tlsConn.Handshake(); err != nil {
		t.Fatalf("Handshake: %v", err)
	}

	return tlsConn.ConnectionState().Version, firstServerHelloRandom(t, rec.buf.Bytes())
}

// TestCCMCapLegacyVersionNegotiatesTLS12 is core-go #125 amended criterion
// 3.4 (CCM half), and the fork-specific regression LOW-5 tracks as core-go
// #130: a ClientHello with legacy_version 0x0304 and no supported_versions
// extension gets a TLS 1.2 ServerHello per RFC 8446 section 4.2.1. RED at
// 406baef: the fork aborted this hello with illegal_parameter because its
// uncapped MaxVersion let the legacy_version field alone imply an attempt
// at TLS 1.3. The TLS 1.2 cap masks the fork defect rather than fixing it
// (LOW-5): capping MaxVersion removes the fork's only path to reach that
// code, so this test passes as a side effect, not because #130 is fixed.
func TestCCMCapLegacyVersionNegotiatesTLS12(t *testing.T) {
	files := newCCMTestFiles(t)
	cfg := newCCMServerConfig(t, files)
	addr, shutdown := startCCMTestListener(t, cfg, nil)
	defer shutdown()

	hello := buildRawClientHelloNoSupportedVersions(0x0304)
	ct, payload := dialRawAndReadFirstRecord(t, addr, hello)

	if ct == 21 {
		t.Fatalf("server sent alert level=%d desc=%d instead of a TLS 1.2 ServerHello", payload[0], payload[1])
	}
	if ct != 22 || len(payload) < 6 || payload[0] != 0x02 {
		t.Fatalf("expected a ServerHello handshake record, got content type %d payload %x", ct, payload)
	}
	negotiated := uint16(payload[4])<<8 | uint16(payload[5])
	if negotiated != 0x0303 {
		t.Errorf("ServerHello.legacy_version = 0x%04x, want TLS 1.2 (0x0303)", negotiated)
	}
}

// TestMutualAuthStillEnforcedOnCCMPath proves the TLS 1.2 cap did not open
// an unauthenticated hole on the CCM/gotls listener path: a client
// presenting no certificate must still be rejected. Covers #125 amended
// criterion 3.5 for the CCM constructor.
func TestMutualAuthStillEnforcedOnCCMPath(t *testing.T) {
	files := newCCMTestFiles(t)
	cfg := newCCMServerConfig(t, files)
	addr, shutdown := startCCMTestListener(t, cfg, nil)
	defer shutdown()

	conn, err := dialCCM(t, addr, files, gotls.VersionTLS12, gotls.VersionTLS12, []uint16{gotls.TLS_ECDHE_ECDSA_WITH_AES_128_CCM_8}, false)
	if err == nil {
		_ = conn.Close()
		t.Fatal("expected handshake to fail without a client cert, it succeeded")
	}
}
