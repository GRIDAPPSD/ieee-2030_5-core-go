package notify

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// TestHandler_NilPeerCert_Returns500 drives the handler directly via
// httptest with a request whose context carries no *gotls.Conn. This
// simulates a non-gotls connection, a plain-HTTP test helper, or a
// future refactor that drops the ConnContext wiring. The handler must
// return 500 and must NOT invoke the dispatcher, enforcing the
// peerCert-is-never-nil contract documented on Dispatcher.
func TestHandler_NilPeerCert_Returns500(t *testing.T) {
	t.Parallel()

	var fired atomic.Int32
	r := &Receiver{
		dispatcher: func(_ context.Context, _ *x509.Certificate, _ sep2.Notification) {
			fired.Add(1)
		},
		// logger is nil: silent path exercised here; the logger branch is
		// covered by inspection since wiring a logger does not change the
		// 500 outcome.
	}

	// Build a minimal valid Notification body so the handler passes all
	// preceding checks (method, content-type, body size, XML decode) and
	// reaches the peer-cert guard rather than returning 400 first.
	n := sep2.Notification{
		Resource:           sep2.Resource{Href: "/notify"},
		SubscribedResource: "/edev/0/fsa",
		SubscriptionURI:    "/edev/0/sub/1",
		NewResourceURI:     "/edev/0/fsa/1",
		Status:             0,
	}
	body, err := xml.Marshal(&n)
	if err != nil {
		t.Fatalf("xml.Marshal: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/notify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/sep+xml")
	// No gotlsConnKey{} stored in req.Context(): the extraction block
	// leaves peerCert nil, triggering the fail-closed guard.

	rw := httptest.NewRecorder()
	r.handler()(rw, req)

	if rw.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d (body: %q)", rw.Code, rw.Body.String())
	}
	if n := fired.Load(); n != 0 {
		t.Fatalf("dispatcher called %d time(s) with nil cert, want 0", n)
	}
}
