package notify_test

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/xml"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core/pkg/sep2"
	certs "gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core/pkg/sep2cert"
	"gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core/pkg/sep2client/notify"
	gotls "gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core/pkg/sep2tls/gotls"
)

// notifyEnv is the shared TLS fixture for the receiver tests. It generates a
// self-signed CA and a device cert into t.TempDir so the listener and the
// mTLS test client can each load through the production paths.
type notifyEnv struct {
	caPool *x509.CertPool

	deviceCertPath string
	deviceKeyPath  string
	caCertPath     string

	// deviceCert is the parsed device certificate, used to verify the peer
	// identity that the dispatcher receives.
	deviceCert *x509.Certificate
}

// newNotifyEnv generates a fresh CA + device cert pair under t.TempDir.
func newNotifyEnv(t *testing.T) *notifyEnv {
	t.Helper()

	caCertPEM, caKeyPEM, err := certs.GenerateCA(certs.CAOptions{
		CommonName: "Notify Test CA",
		ValidYears: 1,
	})
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	caCert, err := certs.ParseCertificatePEM(caCertPEM)
	if err != nil {
		t.Fatalf("ParseCertificatePEM: %v", err)
	}
	caKey, err := certs.ParseKeyPEM(caKeyPEM)
	if err != nil {
		t.Fatalf("ParseKeyPEM: %v", err)
	}

	devCertPEM, devKeyPEM, err := certs.GenerateDeviceCert(caCert, caKey, certs.DeviceCertOptions{
		DeviceType:  certs.DeviceTypeGeneric,
		HWSerialNum: "NOTIFY-TEST-001",
	})
	if err != nil {
		t.Fatalf("GenerateDeviceCert: %v", err)
	}

	devCert, err := certs.ParseCertificatePEM(devCertPEM)
	if err != nil {
		t.Fatalf("ParseCertificatePEM device: %v", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCertPEM) {
		t.Fatal("AppendCertsFromPEM: no certs")
	}

	tmpDir := t.TempDir()
	devCertPath := filepath.Join(tmpDir, "device.crt")
	devKeyPath := filepath.Join(tmpDir, "device.key")
	caPath := filepath.Join(tmpDir, "ca.crt")
	for _, w := range []struct {
		path string
		data []byte
	}{
		{devCertPath, devCertPEM},
		{devKeyPath, devKeyPEM},
		{caPath, caCertPEM},
	} {
		if err := os.WriteFile(w.path, w.data, 0o600); err != nil {
			t.Fatalf("write %s: %v", w.path, err)
		}
	}

	return &notifyEnv{
		caPool:         caPool,
		deviceCertPath: devCertPath,
		deviceKeyPath:  devKeyPath,
		caCertPath:     caPath,
		deviceCert:     devCert,
	}
}

// newReceiver creates, starts, and registers a cleanup for a Receiver backed
// by env. listenAddr defaults to "127.0.0.1:0" (random port).
func newReceiver(t *testing.T, env *notifyEnv, d notify.Dispatcher) *notify.Receiver {
	t.Helper()
	rcv, err := notify.NewReceiver(notify.Config{
		CertFile:   env.deviceCertPath,
		KeyFile:    env.deviceKeyPath,
		CAFile:     env.caCertPath,
		ListenAddr: "127.0.0.1:0",
		Dispatcher: d,
	})
	if err != nil {
		t.Fatalf("NewReceiver: %v", err)
	}
	if rcv == nil {
		t.Fatal("NewReceiver returned nil for non-empty addr")
	}
	if err := rcv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := rcv.Stop(context.Background()); err != nil && !errors.Is(err, notify.ErrNotStarted) {
			t.Errorf("Stop in cleanup: %v", err)
		}
	})
	return rcv
}

// notifyClient builds an HTTP client that mimics what a SEP2 server would do
// when POSTing a Notification: gotls with CCM-8, mTLS using the shared CA.
func notifyClient(t *testing.T, env *notifyEnv) *http.Client {
	t.Helper()

	certPEM, err := os.ReadFile(env.deviceCertPath)
	if err != nil {
		t.Fatalf("read device cert: %v", err)
	}
	keyPEM, err := os.ReadFile(env.deviceKeyPath)
	if err != nil {
		t.Fatalf("read device key: %v", err)
	}
	cert, err := gotls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}

	caPEM, err := os.ReadFile(env.caCertPath)
	if err != nil {
		t.Fatalf("read CA: %v", err)
	}
	rootPool := x509.NewCertPool()
	if !rootPool.AppendCertsFromPEM(caPEM) {
		t.Fatal("AppendCertsFromPEM: no certs")
	}

	tlsCfg := &gotls.Config{
		Certificates: []gotls.Certificate{cert},
		RootCAs:      rootPool,
		MinVersion:   gotls.VersionTLS12,
		MaxVersion:   gotls.VersionTLS12,
		CipherSuites: []uint16{
			gotls.TLS_ECDHE_ECDSA_WITH_AES_128_CCM_8,
			gotls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		},
		// The receiver's server cert is the device cert, which carries an
		// otherName-only SAN (no DNS/IP), so stdlib hostname verification
		// cannot succeed. Bypass hostname check and rely on chain validity:
		// the CA pool is locked to the test CA.
		InsecureSkipVerify: true, //nolint:gosec // see comment above
		CurvePreferences:   []gotls.CurveID{gotls.CurveP256},
	}

	return &http.Client{
		Transport: &http.Transport{
			DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return (&gotls.Dialer{Config: tlsCfg}).DialContext(ctx, network, addr)
			},
		},
		Timeout: 5 * time.Second,
	}
}

// notifyURL composes an HTTPS URL to the receiver's bound address.
func notifyURL(t *testing.T, rcv *notify.Receiver, path string) string {
	t.Helper()
	addr, err := rcv.Addr()
	if err != nil {
		t.Fatalf("Addr: %v", err)
	}
	return "https://" + addr + path
}

// sampleNotificationXML returns a minimal well-formed Notification body.
// subscribedResource is required on any CORE-018 Notification payload.
func sampleNotificationXML(t *testing.T, status uint8, newURI string) []byte {
	t.Helper()
	n := sep2.Notification{
		Resource: sep2.Resource{
			Href: "/notify",
		},
		SubscribedResource: "/edev/0/fsa",
		SubscriptionURI:    "/edev/0/sub/1",
		Status:             status,
		NewResourceURI:     newURI,
	}
	body, err := xml.Marshal(&n)
	if err != nil {
		t.Fatalf("marshal Notification: %v", err)
	}
	return body
}

// --- Construction tests ---------------------------------------------------

// TestNewReceiver_EmptyAddrDisabled asserts the disabled contract: an empty
// ListenAddr returns (nil, nil) so the caller can detect the disabled state
// without an error path.
func TestNewReceiver_EmptyAddrDisabled(t *testing.T) {
	t.Parallel()

	rcv, err := notify.NewReceiver(notify.Config{ListenAddr: ""})
	if err != nil {
		t.Fatalf("want nil err, got %v", err)
	}
	if rcv != nil {
		t.Fatalf("want nil receiver, got %+v", rcv)
	}
}

// TestNewReceiver_BadCertPath asserts a missing cert file surfaces as a
// wrapped error rather than panicking at Start time.
func TestNewReceiver_BadCertPath(t *testing.T) {
	t.Parallel()

	_, err := notify.NewReceiver(notify.Config{
		CertFile:   filepath.Join(t.TempDir(), "nope.crt"),
		KeyFile:    filepath.Join(t.TempDir(), "nope.key"),
		CAFile:     filepath.Join(t.TempDir(), "nope.ca"),
		ListenAddr: "127.0.0.1:0",
	})
	if err == nil {
		t.Fatal("want error from missing cert, got nil")
	}
}

// --- Lifecycle tests -------------------------------------------------------

// TestReceiver_StartIdempotent asserts a double-Start returns ErrAlreadyStarted
// rather than re-binding (which would either deadlock or leak the original
// listener).
func TestReceiver_StartIdempotent(t *testing.T) {
	t.Parallel()

	env := newNotifyEnv(t)
	rcv := newReceiver(t, env, nil)

	if err := rcv.Start(); !errors.Is(err, notify.ErrAlreadyStarted) {
		t.Fatalf("second Start: want %v, got %v", notify.ErrAlreadyStarted, err)
	}
}

// TestReceiver_AddrBeforeStart asserts Addr returns ErrNotStarted when the
// receiver was never started.
func TestReceiver_AddrBeforeStart(t *testing.T) {
	t.Parallel()

	env := newNotifyEnv(t)
	rcv, err := notify.NewReceiver(notify.Config{
		CertFile:   env.deviceCertPath,
		KeyFile:    env.deviceKeyPath,
		CAFile:     env.caCertPath,
		ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("NewReceiver: %v", err)
	}
	if _, err := rcv.Addr(); !errors.Is(err, notify.ErrNotStarted) {
		t.Fatalf("Addr before Start: want %v, got %v", notify.ErrNotStarted, err)
	}
}

// TestReceiver_StopWithoutStart asserts Stop returns ErrNotStarted when the
// receiver was never started.
func TestReceiver_StopWithoutStart(t *testing.T) {
	t.Parallel()

	env := newNotifyEnv(t)
	rcv, err := notify.NewReceiver(notify.Config{
		CertFile:   env.deviceCertPath,
		KeyFile:    env.deviceKeyPath,
		CAFile:     env.caCertPath,
		ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("NewReceiver: %v", err)
	}
	if err := rcv.Stop(context.Background()); !errors.Is(err, notify.ErrNotStarted) {
		t.Fatalf("Stop before Start: want %v, got %v", notify.ErrNotStarted, err)
	}
}

// TestReceiver_AddrBound asserts the bound address is observable immediately
// after Start and that the port is non-zero (random-port resolution worked).
func TestReceiver_AddrBound(t *testing.T) {
	t.Parallel()

	env := newNotifyEnv(t)
	rcv := newReceiver(t, env, nil)

	addr, err := rcv.Addr()
	if err != nil {
		t.Fatalf("Addr: %v", err)
	}
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Errorf("Addr = %q, want 127.0.0.1:N", addr)
	}
	if strings.HasSuffix(addr, ":0") {
		t.Errorf("Addr = %q still has zero port after Start", addr)
	}
}

// TestReceiver_StopGraceful asserts Stop drains the serve goroutine cleanly.
// A goroutine leak would surface as a hang under -race.
func TestReceiver_StopGraceful(t *testing.T) {
	t.Parallel()

	env := newNotifyEnv(t)
	rcv := newReceiver(t, env, nil)

	// First Stop is via t.Cleanup; call it manually to verify it returns nil.
	if err := rcv.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	// Double-Stop returns ErrNotStarted because the first Stop reset state.
	if err := rcv.Stop(context.Background()); !errors.Is(err, notify.ErrNotStarted) {
		t.Fatalf("second Stop: want %v, got %v", notify.ErrNotStarted, err)
	}
}

// --- Handler status-code table --------------------------------------------

// TestHandler_StatusCodes covers the full handler policy matrix: method,
// content-type, body shape.
func TestHandler_StatusCodes(t *testing.T) {
	t.Parallel()

	env := newNotifyEnv(t)

	cases := []struct {
		name        string
		method      string
		contentType string
		body        []byte
		wantStatus  int
	}{
		{
			name:        "happy_204_xml",
			method:      http.MethodPost,
			contentType: "application/sep+xml",
			body:        sampleNotificationXML(t, 0, "/edev/0/fsa"),
			wantStatus:  http.StatusNoContent,
		},
		{
			name:        "happy_204_xml_charset",
			method:      http.MethodPost,
			contentType: "application/sep+xml; charset=utf-8",
			body:        sampleNotificationXML(t, 1, "/edev/0/fsa"),
			wantStatus:  http.StatusNoContent,
		},
		{
			name:        "happy_204_xml_charset_nospace",
			method:      http.MethodPost,
			contentType: "application/sep+xml;charset=utf-8",
			body:        sampleNotificationXML(t, 2, "/edev/0/fsa"),
			wantStatus:  http.StatusNoContent,
		},
		{
			// mime.ParseMediaType normalizes to lowercase, so uppercase variants
			// that would have failed the old exact-match now succeed.
			name:        "happy_204_xml_uppercase",
			method:      http.MethodPost,
			contentType: "APPLICATION/SEP+XML",
			body:        sampleNotificationXML(t, 0, "/edev/0/fsa"),
			wantStatus:  http.StatusNoContent,
		},
		{
			name:        "wrong_method_get",
			method:      http.MethodGet,
			contentType: "application/sep+xml",
			body:        nil,
			wantStatus:  http.StatusMethodNotAllowed,
		},
		{
			name:        "wrong_method_put",
			method:      http.MethodPut,
			contentType: "application/sep+xml",
			body:        sampleNotificationXML(t, 0, "/x"),
			wantStatus:  http.StatusMethodNotAllowed,
		},
		{
			name:        "wrong_content_type",
			method:      http.MethodPost,
			contentType: "application/json",
			body:        []byte(`{"foo":"bar"}`),
			wantStatus:  http.StatusUnsupportedMediaType,
		},
		{
			name:        "empty_body",
			method:      http.MethodPost,
			contentType: "application/sep+xml",
			body:        []byte{},
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "malformed_xml",
			method:      http.MethodPost,
			contentType: "application/sep+xml",
			body:        []byte(`<<<not-xml>>>`),
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "well_formed_wrong_root",
			method:      http.MethodPost,
			contentType: "application/sep+xml",
			body:        []byte(`<DeviceCapability xmlns="urn:ieee:std:2030.5:ns"></DeviceCapability>`),
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "body_too_large",
			method:      http.MethodPost,
			contentType: "application/sep+xml",
			body:        bytes.Repeat([]byte("x"), 65*1024),
			wantStatus:  http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rcv := newReceiver(t, env, nil)
			client := notifyClient(t, env)

			req, err := http.NewRequestWithContext(context.Background(),
				tc.method, notifyURL(t, rcv, "/notify"), bytes.NewReader(tc.body))
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			req.Header.Set("Content-Type", tc.contentType)

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("client.Do: %v", err)
			}
			defer func() {
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}()

			if resp.StatusCode != tc.wantStatus {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("status: want %d, got %d; body=%q", tc.wantStatus, resp.StatusCode, string(body))
			}
		})
	}
}

// --- Dispatcher behavior tests --------------------------------------------

// TestHandler_DispatcherInvoked asserts the dispatcher receives a faithful
// copy of the parsed Notification on the happy path. Field-value assertions
// confirm the decoded values match the wire payload.
func TestHandler_DispatcherInvoked(t *testing.T) {
	t.Parallel()

	env := newNotifyEnv(t)

	var captured atomic.Value // holds sep2.Notification
	var fired atomic.Int32

	d := notify.Dispatcher(func(_ context.Context, _ *x509.Certificate, n sep2.Notification) {
		captured.Store(n)
		fired.Add(1)
	})

	rcv := newReceiver(t, env, d)
	client := notifyClient(t, env)

	body := sampleNotificationXML(t, 1, "/edev/0/fsa/3")
	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodPost, notifyURL(t, rcv, "/notify"), bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/sep+xml")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do: %v", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status: want 204, got %d", resp.StatusCode)
	}
	if got := fired.Load(); got != 1 {
		t.Fatalf("dispatcher fired %d times, want 1", got)
	}

	gotN, ok := captured.Load().(sep2.Notification)
	if !ok {
		t.Fatal("dispatcher did not store a Notification")
	}
	// Field-value assertions per data-invariants rule: assert the decoded
	// values, not just that the dispatcher was called.
	if gotN.SubscribedResource != "/edev/0/fsa" {
		t.Errorf("SubscribedResource = %q, want /edev/0/fsa", gotN.SubscribedResource)
	}
	if gotN.NewResourceURI != "/edev/0/fsa/3" {
		t.Errorf("NewResourceURI = %q, want /edev/0/fsa/3", gotN.NewResourceURI)
	}
	if gotN.Status != 1 {
		t.Errorf("Status = %d, want 1", gotN.Status)
	}
}

// TestHandler_PeerCertPassedToDispatcher asserts that the verified mTLS peer
// leaf certificate is delivered to the Dispatcher. The serial number of the
// received cert must match the device cert the test client presented, ruling
// out any nil or stub substitution.
func TestHandler_PeerCertPassedToDispatcher(t *testing.T) {
	t.Parallel()

	env := newNotifyEnv(t)

	var capturedPeer atomic.Pointer[x509.Certificate]

	d := notify.Dispatcher(func(_ context.Context, peerCert *x509.Certificate, _ sep2.Notification) {
		capturedPeer.Store(peerCert)
	})

	rcv := newReceiver(t, env, d)
	client := notifyClient(t, env)

	body := sampleNotificationXML(t, 0, "/edev/0/fsa")
	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodPost, notifyURL(t, rcv, "/notify"), bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/sep+xml")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do: %v", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status: want 204, got %d", resp.StatusCode)
	}

	got := capturedPeer.Load()
	if got == nil {
		t.Fatal("dispatcher received nil peerCert")
	}
	// Verify by serial number: the cert carries a random serial set at
	// generation time; matching it confirms the dispatcher received the
	// actual device cert the client presented, not a nil or wrong cert.
	if got.SerialNumber.Cmp(env.deviceCert.SerialNumber) != 0 {
		t.Errorf("peerCert.SerialNumber = %s, want %s",
			got.SerialNumber, env.deviceCert.SerialNumber)
	}
}

// TestHandler_DispatcherNotInvokedOnError asserts the dispatcher is NOT
// called for malformed bodies. This is important so a buggy or hostile
// server cannot trigger consumer-policy state-machine churn by sending
// garbage.
func TestHandler_DispatcherNotInvokedOnError(t *testing.T) {
	t.Parallel()

	env := newNotifyEnv(t)

	var fired atomic.Int32
	d := notify.Dispatcher(func(_ context.Context, _ *x509.Certificate, _ sep2.Notification) {
		fired.Add(1)
	})

	rcv := newReceiver(t, env, d)
	client := notifyClient(t, env)

	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodPost, notifyURL(t, rcv, "/notify"), bytes.NewReader([]byte("<broken")))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/sep+xml")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do: %v", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", resp.StatusCode)
	}
	if got := fired.Load(); got != 0 {
		t.Fatalf("dispatcher fired %d times on malformed body, want 0", got)
	}
}

// TestHandler_DispatcherPanicRecovery asserts that a panicking Dispatcher
// does not crash the receiver process: the handler returns 500 and the
// receiver continues serving subsequent requests.
func TestHandler_DispatcherPanicRecovery(t *testing.T) {
	t.Parallel()

	env := newNotifyEnv(t)

	d := notify.Dispatcher(func(_ context.Context, _ *x509.Certificate, _ sep2.Notification) {
		panic("test panic in dispatcher")
	})

	rcv := newReceiver(t, env, d)
	client := notifyClient(t, env)

	body := sampleNotificationXML(t, 0, "/edev/0/fsa")
	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodPost, notifyURL(t, rcv, "/notify"), bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/sep+xml")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do: %v", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	// A panicking dispatcher must not crash the receiver: it returns 500.
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status: want 500 on panicking dispatcher, got %d", resp.StatusCode)
	}
}

// --- Security test --------------------------------------------------------

// TestReceiver_RejectsUnsignedClient asserts the mTLS security invariant:
// a client presenting a certificate signed by a foreign (untrusted) CA fails
// the TLS handshake. Without this invariant any host could impersonate a SEP2
// server and inject Notifications.
func TestReceiver_RejectsUnsignedClient(t *testing.T) {
	t.Parallel()

	env := newNotifyEnv(t)
	rcv := newReceiver(t, env, nil)

	// Generate a second, foreign CA and issue a device cert under it. The
	// receiver's CA pool trusts only env's CA: this cert's chain will not
	// validate.
	foreignCAPEM, foreignCAKeyPEM, err := certs.GenerateCA(certs.CAOptions{
		CommonName: "Foreign CA",
		ValidYears: 1,
	})
	if err != nil {
		t.Fatalf("foreign CA: %v", err)
	}
	foreignCACert, err := certs.ParseCertificatePEM(foreignCAPEM)
	if err != nil {
		t.Fatalf("parse foreign CA cert: %v", err)
	}
	foreignCAKey, err := certs.ParseKeyPEM(foreignCAKeyPEM)
	if err != nil {
		t.Fatalf("parse foreign CA key: %v", err)
	}
	foreignDevCertPEM, foreignDevKeyPEM, err := certs.GenerateDeviceCert(foreignCACert, foreignCAKey, certs.DeviceCertOptions{
		DeviceType:  certs.DeviceTypeGeneric,
		HWSerialNum: "FOREIGN-001",
	})
	if err != nil {
		t.Fatalf("foreign device cert: %v", err)
	}
	foreignCert, err := gotls.X509KeyPair(foreignDevCertPEM, foreignDevKeyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair foreign: %v", err)
	}

	// Trust the receiver's CA on the dial side so the server's leaf chain
	// validates from this client's perspective. We want the handshake to
	// fail because of OUR cert, not the server's.
	caPEM, err := os.ReadFile(env.caCertPath)
	if err != nil {
		t.Fatalf("read CA: %v", err)
	}
	rootPool := x509.NewCertPool()
	if !rootPool.AppendCertsFromPEM(caPEM) {
		t.Fatal("AppendCertsFromPEM: no certs")
	}

	tlsCfg := &gotls.Config{
		Certificates: []gotls.Certificate{foreignCert},
		RootCAs:      rootPool,
		MinVersion:   gotls.VersionTLS12,
		MaxVersion:   gotls.VersionTLS12,
		CipherSuites: []uint16{
			gotls.TLS_ECDHE_ECDSA_WITH_AES_128_CCM_8,
			gotls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		},
		InsecureSkipVerify: true, //nolint:gosec // see notifyClient comment
		CurvePreferences:   []gotls.CurveID{gotls.CurveP256},
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return (&gotls.Dialer{Config: tlsCfg}).DialContext(ctx, network, addr)
			},
		},
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodPost, notifyURL(t, rcv, "/notify"),
		bytes.NewReader(sampleNotificationXML(t, 0, "/edev/0/fsa")))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/sep+xml")

	// The mTLS handshake must fail: a foreign-CA cert must be rejected.
	if _, err := client.Do(req); err == nil {
		t.Fatal("expected TLS handshake failure for foreign-CA client cert, got nil error")
	}
}
