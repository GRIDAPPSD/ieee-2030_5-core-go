package notify_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// lockedBuf is a log destination safe for concurrent writes and reads that
// signals on written after every write.
type lockedBuf struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	written chan struct{}
}

func (b *lockedBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	n, err := b.buf.Write(p)
	b.mu.Unlock()
	select {
	case b.written <- struct{}{}:
	default:
	}
	return n, err
}

func (b *lockedBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestReceiver_LogsRefusedHandshake proves a client refused at the TLS
// handshake leaves one line in the server log, as net/http logs a refused
// handshake on a crypto/tls listener, and that the receiver still serves and
// stops normally afterwards. Not parallel: it redirects the standard logger.
func TestReceiver_LogsRefusedHandshake(t *testing.T) {
	logBuf := &lockedBuf{written: make(chan struct{}, 1)}
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(logBuf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	}()

	env := newNotifyEnv(t)
	rcv := newReceiver(t, env, nil)
	addr, err := rcv.Addr()
	if err != nil {
		t.Fatalf("Addr: %v", err)
	}

	tls13Only := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		MaxVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true, //nolint:gosec // the refusal happens before any certificate is checked
	}
	if conn, err := tls.Dial("tcp", addr, tls13Only); err == nil {
		_ = conn.Close()
		t.Fatal("expected a TLS 1.3-only client to be refused, the dial succeeded")
	}

	select {
	case <-logBuf.written:
	case <-time.After(2 * time.Second):
		t.Fatal("no server log line within 2s of the refused handshake")
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		notifyURL(t, rcv, "/notify"), bytes.NewReader(sampleNotificationXML(t, 0, "/edev/0/fsa")))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/sep+xml")
	resp, err := notifyClient(t, env).Do(req)
	if err != nil {
		t.Fatalf("POST after the refusal: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("POST after the refusal: status %d, want 204", resp.StatusCode)
	}

	stopped := make(chan error, 1)
	go func() { stopped <- rcv.Stop(context.Background()) }()
	select {
	case err := <-stopped:
		if err != nil {
			t.Errorf("Stop: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Stop did not return within 3s")
	}

	logged := logBuf.String()
	if n := strings.Count(logged, "TLS handshake error"); n != 1 {
		t.Errorf("server log = %q, want exactly one TLS handshake error line, got %d", logged, n)
	}
	if !strings.Contains(logged, "unsupported versions") {
		t.Errorf("server log = %q, want the refusal cause (unsupported versions)", logged)
	}
}
