package sep2tls_test

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2cert"
	sepTLS "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls"
)

// tls13CertSet mints a CA, a server cert, and a device (client) cert for the
// TLS-version-acceptance tests below. Mirrors newTestCertSet-style helpers
// already proven in config_test.go and pkg/sep2srv/server_test.go.
type tls13CertSet struct {
	caPEM     []byte
	serverPEM []byte
	serverKey []byte
	devicePEM []byte
	deviceKey []byte
}

func newTLS13CertSet(t *testing.T) tls13CertSet {
	t.Helper()

	caCertPEM, caKeyPEM, err := sep2cert.GenerateCA(sep2cert.CAOptions{
		CommonName: "TLS13 Test CA",
		ValidYears: 1,
	})
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	caCert, caKey := parseCACert(t, caCertPEM, caKeyPEM)

	serverCertPEM, serverKeyPEM, err := sep2cert.GenerateServerCert(caCert, caKey, sep2cert.ServerCertOptions{
		Hosts:      []string{"127.0.0.1", "localhost"},
		CommonName: "TLS13 Test Server",
		ValidYears: 1,
	})
	if err != nil {
		t.Fatalf("GenerateServerCert: %v", err)
	}

	deviceCertPEM, deviceKeyPEM, err := sep2cert.GenerateDeviceCert(caCert, caKey, sep2cert.DeviceCertOptions{
		DeviceType:  sep2cert.DeviceTypeGeneric,
		HWSerialNum: "TLS13-TEST-001",
	})
	if err != nil {
		t.Fatalf("GenerateDeviceCert: %v", err)
	}

	return tls13CertSet{
		caPEM:     caCertPEM,
		serverPEM: serverCertPEM,
		serverKey: serverKeyPEM,
		devicePEM: deviceCertPEM,
		deviceKey: deviceKeyPEM,
	}
}

// startTLS13TestServer stands up an HTTPS listener using
// sepTLS.NewServerTLSConfigFromPEM (the same constructor NewServerTLSConfig
// and NewServerTLSConfigWithExtraCAs delegate to) and returns its address
// plus a shutdown func. The handler echoes the negotiated TLS version so
// tests can assert on it directly.
func startTLS13TestServer(t *testing.T, certs tls13CertSet) (addr string, shutdown func()) {
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

// dialWithVersion performs an HTTPS GET against addr using a client pinned
// to exactly minVer/maxVer (both set to the same value simulates a
// version-only client, mirroring the EPRI reference client which offers
// only TLS 1.3). withCert controls whether the client presents its device
// certificate.
func dialWithVersion(t *testing.T, addr string, certs tls13CertSet, minVer, maxVer uint16, withCert bool) (*http.Response, error) {
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
		// InsecureSkipVerify is only needed because we are not asserting
		// server identity here; the point of this branch is what happens
		// on OUR side (client-cert requirement), not server-cert trust.
		clientTLSCfg.InsecureSkipVerify = true
	}

	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: clientTLSCfg},
	}

	return client.Get("https://" + addr + "/test")
}

// TestTLS12StillWorksAfterTLS13Widening proves the spec floor (TLS 1.2 +
// ECDHE-ECDSA-AES128-GCM under the crypto/tls fallback path) still
// negotiates exactly as before MaxVersion was raised to 1.3. A regression
// here would mean a spec-strict CSIP 1.2-only client can no longer connect.
func TestTLS12StillWorksAfterTLS13Widening(t *testing.T) {
	certs := newTLS13CertSet(t)
	addr, shutdown := startTLS13TestServer(t, certs)
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

// TestTLS13OnlyClientNowConnects reproduces the EPRI reference client
// symptom: a client that offers ONLY TLS 1.3 (MinVersion == MaxVersion ==
// VersionTLS13). Before the MaxVersion widening this failed the handshake
// with "no cipher suite supported by both client and server" /
// "client offered only unsupported versions". It must now complete and
// still carry the client cert through to the handler.
func TestTLS13OnlyClientNowConnects(t *testing.T) {
	certs := newTLS13CertSet(t)
	addr, shutdown := startTLS13TestServer(t, certs)
	defer shutdown()

	resp, err := dialWithVersion(t, addr, certs, tls.VersionTLS13, tls.VersionTLS13, true)
	if err != nil {
		t.Fatalf("TLS 1.3-only client GET failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200, body: %s", resp.StatusCode, body)
	}
	if resp.TLS.Version != tls.VersionTLS13 {
		t.Errorf("negotiated version = %s, want TLS 1.3", tls.VersionName(resp.TLS.Version))
	}
	if len(resp.TLS.PeerCertificates) == 0 {
		t.Error("server reported no client cert on a TLS 1.3 handshake with a presented device cert")
	}
	if got := resp.Header.Get("X-TLS-Version"); got != "TLS 1.3" {
		t.Errorf("handler observed r.TLS.Version = %q, want %q", got, "TLS 1.3")
	}
}

// TestMutualAuthStillEnforcedUnderBothVersions proves the TLS 1.3 widening
// did not open an unauthenticated hole: a client presenting no certificate
// must still be rejected at the handshake (RequireAnyClientCert), under
// BOTH TLS 1.2 and TLS 1.3.
func TestMutualAuthStillEnforcedUnderBothVersions(t *testing.T) {
	tests := []struct {
		name string
		vers uint16
	}{
		{"TLS 1.2", tls.VersionTLS12},
		{"TLS 1.3", tls.VersionTLS13},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			certs := newTLS13CertSet(t)
			addr, shutdown := startTLS13TestServer(t, certs)
			defer shutdown()

			_, err := dialWithVersion(t, addr, certs, tt.vers, tt.vers, false)
			if err == nil {
				t.Fatalf("expected handshake to fail without a client cert under %s, it succeeded", tt.name)
			}
		})
	}
}
