package sep2tls

import "time"

// SetCCMHandshakeTimeoutForTest overrides the CCM listener's handshake bound
// for the life of a test; the returned func restores the prior value. This
// file is excluded from non-test builds, so it adds no public API.
func SetCCMHandshakeTimeoutForTest(d time.Duration) (restore func()) {
	prev := ccmHandshakeTimeout
	ccmHandshakeTimeout = d
	return func() { ccmHandshakeTimeout = prev }
}
