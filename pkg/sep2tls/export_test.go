package sep2tls

import (
	"net"
	"time"
)

// CCMHandshakeTimeoutOf reports the handshake bound a listener returned by
// WrapCCMListener or WrapCCMListenerWithTimeout is running with. This file is
// excluded from non-test builds, so it adds no public API.
func CCMHandshakeTimeoutOf(l net.Listener) time.Duration {
	return l.(*ccmLoggingListener).handshakeTimeout
}
