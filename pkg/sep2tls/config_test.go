package sep2tls_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2cert"
	sepTLS "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls"
	gotls "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls/gotls"
)

// TestMutualTLSHandshake proves the end-to-end wiring an http.Server /
// http.Client pair needs on the fork: SetupCCMServer plus
// CCMIdentityMiddleware to populate r.TLS on the server side, and a
// DialTLSContext hook built from gotls.Dialer on the client side, since
// neither happens automatically the way it does for a stdlib *tls.Conn.
func TestMutualTLSHandshake(t *testing.T) {
	// Generate CA, server cert, and device cert
	caCertPEM, caKeyPEM, err := sep2cert.GenerateCA(sep2cert.CAOptions{
		CommonName: "Test CA",
		ValidYears: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	caCert, caKey := parseCACert(t, caCertPEM, caKeyPEM)

	serverCertPEM, serverKeyPEM, err := sep2cert.GenerateServerCert(caCert, caKey, sep2cert.ServerCertOptions{
		Hosts:      []string{"127.0.0.1", "localhost"},
		CommonName: "Test Server",
		ValidYears: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	deviceCertPEM, deviceKeyPEM, err := sep2cert.GenerateDeviceCert(caCert, caKey, sep2cert.DeviceCertOptions{
		DeviceType:  sep2cert.DeviceTypeGeneric,
		HWSerialNum: "TEST-001",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create server TLS config (CCM-8 only, fork)
	serverTLSCfg, err := sepTLS.NewCCMServerConfigFromPEM(serverCertPEM, serverKeyPEM, caCertPEM)
	if err != nil {
		t.Fatal(err)
	}

	// Create client TLS config (CCM-8 only, fork)
	clientTLSCfg, err := sepTLS.NewCCMClientConfigFromPEM(deviceCertPEM, deviceKeyPEM, caCertPEM)
	if err != nil {
		t.Fatal(err)
	}

	// Start TLS server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()

	tlsListener := gotls.NewListener(listener, serverTLSCfg)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify we received the client certificate
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "no client cert", http.StatusForbidden)
			return
		}

		cert := r.TLS.PeerCertificates[0]
		sfdi := sepTLS.SFDI(cert)
		lfdi := sepTLS.LFDI(cert)

		_, _ = fmt.Fprintf(w, "SFDI=%s LFDI=%s", sfdi, lfdi)
	})

	srv := &http.Server{Handler: sepTLS.CCMIdentityMiddleware(handler)}
	sepTLS.SetupCCMServer(srv)
	go func() { _ = srv.Serve(tlsListener) }()
	defer func() { _ = srv.Close() }()

	// Make client request over the fork's dialer, wired as DialTLSContext:
	// http.Transport.TLSClientConfig only accepts a *tls.Config, and stdlib
	// crypto/tls cannot negotiate CCM-8 at all.
	client := &http.Client{
		Transport: &http.Transport{
			DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return (&gotls.Dialer{Config: clientTLSCfg}).DialContext(ctx, network, addr)
			},
		},
	}

	addr := listener.Addr().String()
	resp, err := client.Get("https://" + addr + "/test")
	if err != nil {
		t.Fatalf("client GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if len(bodyStr) == 0 {
		t.Fatal("empty response body")
	}

	// Verify SFDI and LFDI are present
	if !containsSubstring(bodyStr, "SFDI=") || !containsSubstring(bodyStr, "LFDI=") {
		t.Errorf("response should contain SFDI and LFDI, got: %s", bodyStr)
	}

	t.Logf("mutual TLS response: %s", bodyStr)

	// net/http never populates resp.TLS for a DialTLSContext connection that
	// is not a *crypto/tls.Conn (documented on NewCCMClientConfig), so the
	// cipher suite is not readable from resp.TLS; it stays nil here. Risk
	// area 2 (negotiated suite read from the connection state) is covered
	// by ccm_tls12_cap_test.go's TestCCM8StillWorksUnderCap, which reads it
	// from the dialed *gotls.Conn directly.
	if resp.TLS != nil {
		t.Errorf("resp.TLS = %+v, want nil: net/http does not recognize *gotls.Conn", resp.TLS)
	}
}

// TestTLSRejectsNoClientCert proves the fork server config still enforces
// mutual auth: a client presenting no certificate is refused at handshake.
func TestTLSRejectsNoClientCert(t *testing.T) {
	caCertPEM, caKeyPEM, _ := sep2cert.GenerateCA(sep2cert.CAOptions{
		CommonName: "Test CA",
		ValidYears: 1,
	})

	caCert, caKey := parseCACert(t, caCertPEM, caKeyPEM)

	serverCertPEM, serverKeyPEM, _ := sep2cert.GenerateServerCert(caCert, caKey, sep2cert.ServerCertOptions{
		Hosts: []string{"127.0.0.1"},
	})

	serverTLSCfg, _ := sepTLS.NewCCMServerConfigFromPEM(serverCertPEM, serverKeyPEM, caCertPEM)

	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	defer func() { _ = listener.Close() }()

	tlsListener := gotls.NewListener(listener, serverTLSCfg)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})}
	go func() { _ = srv.Serve(tlsListener) }()
	defer func() { _ = srv.Close() }()

	// Client WITHOUT a certificate
	client := &http.Client{
		Transport: &http.Transport{
			DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				//nolint:gosec // no client cert on purpose: exercises the mutual-auth-required rejection path
				cfg := &gotls.Config{InsecureSkipVerify: true}
				return (&gotls.Dialer{Config: cfg}).DialContext(ctx, network, addr)
			},
		},
	}

	addr := listener.Addr().String()
	_, err := client.Get("https://" + addr + "/test")
	if err == nil {
		t.Error("expected TLS handshake to fail without client cert")
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsAt(s, sub))
}

func containsAt(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func parseCACert(t *testing.T, certPEM, keyPEM []byte) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("failed to decode cert PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		t.Fatal("failed to decode key PEM")
	}
	keyRaw, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		t.Fatalf("ParsePKCS8PrivateKey: %v", err)
	}
	ecKey, ok := keyRaw.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatal("CA key is not ECDSA")
	}
	return cert, ecKey
}
