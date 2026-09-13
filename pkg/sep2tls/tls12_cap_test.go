package sep2tls_test

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2cert"
	sepTLS "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls"
)

// capCertSet mints a CA, a server cert, and a device (client) cert for the
// TLS 1.2 cap tests below (core-go #125).
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

// startCapTestServer stands up an HTTPS listener using
// sepTLS.NewServerTLSConfigFromPEM and returns its address plus a shutdown
// func. The handler echoes the negotiated TLS version so tests can assert on
// it directly.
func startCapTestServer(t *testing.T, certs capCertSet) (addr string, shutdown func()) {
	t.Helper()

	serverTLSCfg, err := sepTLS.NewServerTLSConfigFromPEM(certs.serverPEM, certs.serverKey, certs.caPEM)
	if err != nil {
		t.Fatalf("NewServerTLSConfigFromPEM: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	tlsListener := tls.NewListener(listener, serverTLSCfg)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "no client cert", http.StatusForbidden)
			return
		}
		w.Header().Set("X-TLS-Version", tls.VersionName(r.TLS.Version))
		_, _ = io.WriteString(w, "ok")
	})

	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(tlsListener) }()

	return listener.Addr().String(), func() { _ = srv.Close() }
}

// startCapRawListener stands up a raw tls.Listener using cfg (bypassing
// net/http) and returns each Accept's handshake error on errs, so a test can
// assert on the server-side refusal reason rather than only the client's.
func startCapRawListener(t *testing.T, cfg *tls.Config) (addr string, errs chan error, shutdown func()) {
	t.Helper()

	listener, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatalf("tls.Listen: %v", err)
	}

	errCh := make(chan error, 4)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				tc, ok := c.(*tls.Conn)
				if !ok {
					return
				}
				if err := tc.Handshake(); err != nil {
					select {
					case errCh <- err:
					default:
					}
				}
			}(conn)
		}
	}()

	return listener.Addr().String(), errCh, func() { _ = listener.Close() }
}

// dialWithVersion performs an HTTPS GET against addr using a client pinned to
// exactly minVer/maxVer. withCert controls whether the client presents its
// device certificate.
func dialWithVersion(t *testing.T, addr string, certs capCertSet, minVer, maxVer uint16, withCert bool) (*http.Response, error) {
	t.Helper()

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(certs.caPEM) {
		t.Fatal("failed to parse CA cert into pool")
	}

	clientTLSCfg := &tls.Config{
		RootCAs:          caPool,
		MinVersion:       minVer,
		MaxVersion:       maxVer,
		CurvePreferences: []tls.CurveID{tls.CurveP256},
	}

	if withCert {
		deviceCert, err := tls.X509KeyPair(certs.devicePEM, certs.deviceKey)
		if err != nil {
			t.Fatalf("X509KeyPair (device cert): %v", err)
		}
		clientTLSCfg.Certificates = []tls.Certificate{deviceCert}
	} else {
		// No client cert: exercise the mutual-auth-required rejection path.
		clientTLSCfg.InsecureSkipVerify = true
	}

	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: clientTLSCfg},
	}

	return client.Get("https://" + addr + "/test")
}

// TestTLS12StillWorksUnderCap proves the spec floor (TLS 1.2 +
// ECDHE-ECDSA-AES128-GCM) still negotiates under the TLS 1.2 cap exactly as
// it did before the cap.
func TestTLS12StillWorksUnderCap(t *testing.T) {
	certs := newCapCertSet(t)
	addr, shutdown := startCapTestServer(t, certs)
	defer shutdown()

	resp, err := dialWithVersion(t, addr, certs, tls.VersionTLS12, tls.VersionTLS12, true)
	if err != nil {
		t.Fatalf("TLS 1.2 client GET failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200, body: %s", resp.StatusCode, body)
	}
	if resp.TLS.Version != tls.VersionTLS12 {
		t.Errorf("negotiated version = %s, want TLS 1.2", tls.VersionName(resp.TLS.Version))
	}
	if resp.TLS.CipherSuite != tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256 {
		t.Errorf("cipher suite = 0x%04x, want GCM 0x%04x", resp.TLS.CipherSuite, tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256)
	}
	if len(resp.TLS.PeerCertificates) == 0 {
		t.Error("server reported no client cert on a TLS 1.2 handshake with a presented device cert")
	}
}

// TestStdCapRefusesTLS13OnlyClient is core-go #125 amended criterion 3.1 for
// the standard-library constructor. Replaces TestTLS13OnlyClientNowConnects,
// withdrawn by the amendment. RED at 406baef: NewServerTLSConfigFromPEM's
// MaxVersion was TLS 1.3 there, so this client completed instead of being
// refused.
func TestStdCapRefusesTLS13OnlyClient(t *testing.T) {
	certs := newCapCertSet(t)
	serverTLSCfg, err := sepTLS.NewServerTLSConfigFromPEM(certs.serverPEM, certs.serverKey, certs.caPEM)
	if err != nil {
		t.Fatalf("NewServerTLSConfigFromPEM: %v", err)
	}
	addr, errs, shutdown := startCapRawListener(t, serverTLSCfg)
	defer shutdown()

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

	conn, dialErr := tls.Dial("tcp", addr, clientCfg)
	if dialErr == nil {
		_ = conn.Close()
		t.Fatal("expected a TLS 1.3-only client to be refused, the handshake succeeded")
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

// TestStdCapNegotiatesTLS12ForDualVersionClient is core-go #125 amended
// criterion 3.2 (standard-library half): a client offering both TLS 1.2 and
// TLS 1.3 negotiates TLS 1.2. RED at 406baef: the uncapped config negotiated
// TLS 1.3, since that was the higher version both sides supported.
func TestStdCapNegotiatesTLS12ForDualVersionClient(t *testing.T) {
	certs := newCapCertSet(t)
	addr, shutdown := startCapTestServer(t, certs)
	defer shutdown()

	resp, err := dialWithVersion(t, addr, certs, tls.VersionTLS12, tls.VersionTLS13, true)
	if err != nil {
		t.Fatalf("dual-version client GET failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.TLS.Version != tls.VersionTLS12 {
		t.Errorf("negotiated version = %s, want TLS 1.2", tls.VersionName(resp.TLS.Version))
	}
}

// TestStdCapNoDowngradeSentinel is core-go #125 amended criterion 3.3
// (standard-library half). It proves two things in one test, so the negative
// assertion can fail: a client capable of TLS 1.3 completes on TLS 1.2
// against the capped config with no RFC 8446 section 4.1.3 downgrade
// sentinel in ServerHello.random, and the same detection code finds the
// sentinel when a TLS 1.3-capable config serves TLS 1.2 to a 1.2-only
// client (the scratch uncapped config below is built only to prove the
// detector fires; core no longer ships a TLS 1.3-capable server config).
func TestStdCapNoDowngradeSentinel(t *testing.T) {
	certs := newCapCertSet(t)

	t.Run("capped config writes no sentinel", func(t *testing.T) {
		serverTLSCfg, err := sepTLS.NewServerTLSConfigFromPEM(certs.serverPEM, certs.serverKey, certs.caPEM)
		if err != nil {
			t.Fatalf("NewServerTLSConfigFromPEM: %v", err)
		}
		addr, _, shutdown := startCapRawListener(t, serverTLSCfg)
		defer shutdown()

		version, random := recordAndHandshake(t, addr, certs.caPEM, certs.devicePEM, certs.deviceKey, tls.VersionTLS12, 0) // MaxVersion 0: client's default, TLS 1.3
		if version != tls.VersionTLS12 {
			t.Fatalf("negotiated version = %s, want TLS 1.2: the sentinel check only means something if the server was actually forced down to 1.2", tls.VersionName(version))
		}
		if tail := random[24:]; string(tail) == string(downgradeCanaryTLS12) {
			t.Errorf("capped config wrote the downgrade sentinel %x, it has nothing to downgrade from", tail)
		}
	})

	t.Run("control: an uncapped config does write the sentinel", func(t *testing.T) {
		serverTLSCfg, err := sepTLS.NewServerTLSConfigFromPEM(certs.serverPEM, certs.serverKey, certs.caPEM)
		if err != nil {
			t.Fatalf("NewServerTLSConfigFromPEM: %v", err)
		}
		serverTLSCfg.MaxVersion = tls.VersionTLS13 // scratch-only: proves the detector, not shipped
		addr, _, shutdown := startCapRawListener(t, serverTLSCfg)
		defer shutdown()

		// A 1.2-only client forces the downgrade path against the 1.3-capable server.
		version, random := recordAndHandshake(t, addr, certs.caPEM, certs.devicePEM, certs.deviceKey, tls.VersionTLS12, tls.VersionTLS12)
		if version != tls.VersionTLS12 {
			t.Fatalf("negotiated version = %s, want TLS 1.2", tls.VersionName(version))
		}
		if got := random[24:]; string(got) != string(downgradeCanaryTLS12) {
			t.Fatalf("control did not observe the downgrade sentinel (got %x): the detector cannot prove a negative", got)
		}
	})
}

// TestStdCapLegacyVersionNegotiatesTLS12 is core-go #125 amended criterion
// 3.4 (standard-library half): a ClientHello with legacy_version 0x0304 and
// no supported_versions extension gets a TLS 1.2 ServerHello per RFC 8446
// section 4.2.1. Not RED at 406baef for this constructor: stdlib crypto/tls
// already implements section 4.2.1 correctly, unlike the fork (LOW-5,
// core-go #130, exercised by the CCM version of this test).
func TestStdCapLegacyVersionNegotiatesTLS12(t *testing.T) {
	certs := newCapCertSet(t)
	serverTLSCfg, err := sepTLS.NewServerTLSConfigFromPEM(certs.serverPEM, certs.serverKey, certs.caPEM)
	if err != nil {
		t.Fatalf("NewServerTLSConfigFromPEM: %v", err)
	}
	addr, _, shutdown := startCapRawListener(t, serverTLSCfg)
	defer shutdown()

	assertLegacyVersionNegotiatesTLS12(t, addr, 0x0304)
}

// TestMutualAuthStillEnforcedUnderCap proves the TLS 1.2 cap did not open an
// unauthenticated hole: a client presenting no certificate is still rejected
// at the handshake (RequireAnyClientCert). Covers #125 amended criterion 3.5
// for the standard-library constructor.
func TestMutualAuthStillEnforcedUnderCap(t *testing.T) {
	certs := newCapCertSet(t)
	addr, shutdown := startCapTestServer(t, certs)
	defer shutdown()

	_, err := dialWithVersion(t, addr, certs, tls.VersionTLS12, tls.VersionTLS12, false)
	if err == nil {
		t.Fatal("expected handshake to fail without a client cert, it succeeded")
	}
}
