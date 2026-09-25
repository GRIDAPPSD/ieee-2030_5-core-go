package sep2tls_test

import (
	"bytes"
	"crypto/rand"
	"crypto/x509"
	"encoding/binary"
	"net"
	"testing"
	"time"

	gotls "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls/gotls"
)

// downgradeCanaryTLS12 is the RFC 8446 section 4.1.3 sentinel a TLS 1.3-capable
// server writes into the last 8 bytes of ServerHello.random when it negotiates
// TLS 1.2 with a peer. A server whose own MaxVersion is capped at TLS 1.2 has
// nothing to downgrade from and must never write it.
var downgradeCanaryTLS12 = []byte("DOWNGRD\x01")

// buildRawClientHelloNoSupportedVersions builds a wire-format TLS ClientHello
// record with the given legacy_version and no supported_versions extension,
// to exercise RFC 8446 section 4.2.1's backward-compatible negotiation path
// (core-go #125 amended criterion 3.4). No stdlib or gotls client can be
// configured to omit that extension while raising legacy_version, so the
// bytes are hand-built.
func buildRawClientHelloNoSupportedVersions(legacyVersion uint16) []byte {
	var random [32]byte
	_, _ = rand.Read(random[:])

	body := new(bytes.Buffer)
	_ = binary.Write(body, binary.BigEndian, legacyVersion)
	body.Write(random[:])
	body.WriteByte(0) // session_id: empty

	cipherSuites := []uint16{0xC0AE, 0xC02B} // CCM_8, then GCM
	_ = binary.Write(body, binary.BigEndian, uint16(len(cipherSuites)*2))
	for _, cs := range cipherSuites {
		_ = binary.Write(body, binary.BigEndian, cs)
	}

	body.WriteByte(1) // compression_methods length
	body.WriteByte(0) // null compression

	ext := new(bytes.Buffer)
	writeExtension(ext, 0x000a, extSupportedGroups())
	writeExtension(ext, 0x000b, extECPointFormats())
	writeExtension(ext, 0x000d, extSignatureAlgorithms())
	_ = binary.Write(body, binary.BigEndian, uint16(ext.Len()))
	body.Write(ext.Bytes())

	hs := new(bytes.Buffer)
	hs.WriteByte(0x01) // ClientHello
	writeUint24(hs, uint32(body.Len()))
	hs.Write(body.Bytes())

	rec := new(bytes.Buffer)
	rec.WriteByte(0x16) // Handshake
	rec.Write([]byte{0x03, 0x01})
	_ = binary.Write(rec, binary.BigEndian, uint16(hs.Len()))
	rec.Write(hs.Bytes())
	return rec.Bytes()
}

func writeExtension(buf *bytes.Buffer, typ uint16, data []byte) {
	_ = binary.Write(buf, binary.BigEndian, typ)
	_ = binary.Write(buf, binary.BigEndian, uint16(len(data)))
	buf.Write(data)
}

func extSupportedGroups() []byte {
	// secp256r1 (0x0017): the only curve these constructors configure.
	return []byte{0x00, 0x02, 0x00, 0x17}
}

func extECPointFormats() []byte {
	return []byte{0x01, 0x00} // list length 1, uncompressed
}

func extSignatureAlgorithms() []byte {
	return []byte{0x00, 0x02, 0x04, 0x03} // ecdsa_secp256r1_sha256
}

func writeUint24(buf *bytes.Buffer, v uint32) {
	buf.WriteByte(byte(v >> 16))
	buf.WriteByte(byte(v >> 8))
	buf.WriteByte(byte(v))
}

// dialRawAndReadFirstRecord opens a raw TCP connection, writes hello, and
// returns the first TLS record the server sends back. It does not complete a
// handshake: the point of every raw-hello test is to inspect what the server
// sends before any client response, not to authenticate.
func dialRawAndReadFirstRecord(t *testing.T, addr string, hello []byte) (contentType byte, payload []byte) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("SetDeadline: %v", err)
	}
	if _, err := conn.Write(hello); err != nil {
		t.Fatalf("write ClientHello: %v", err)
	}

	buf := make([]byte, 8192)
	n, err := conn.Read(buf)
	if n == 0 && err != nil {
		t.Fatalf("read response: %v", err)
	}
	buf = buf[:n]

	if len(buf) < 5 {
		t.Fatalf("response too short for a TLS record header: %d bytes", len(buf))
	}
	length := int(buf[3])<<8 | int(buf[4])
	if len(buf) < 5+length {
		t.Fatalf("truncated TLS record: have %d bytes, header declares %d", len(buf), 5+length)
	}
	return buf[0], buf[5 : 5+length]
}

// recordingConn wraps a net.Conn and copies every byte read from the peer
// into buf, so a test can inspect the raw ServerHello a completed stdlib
// handshake never exposes through tls.ConnectionState.
type recordingConn struct {
	net.Conn
	buf *bytes.Buffer
}

func (r *recordingConn) Read(p []byte) (int, error) {
	n, err := r.Conn.Read(p)
	if n > 0 {
		r.buf.Write(p[:n])
	}
	return n, err
}

// recordAndHandshake performs a gotls (fork) handshake against addr,
// trusting caPEM and presenting the devicePEM/deviceKeyPEM client identity,
// offering CCM-8 (the only suite core's CCM server configs offer), over a
// recordingConn so the raw ServerHello.random is recoverable afterward. Its
// sole caller is the CCM listener suite (core-go #143): a stdlib
// *crypto/tls.Client cannot negotiate CCM-8 at all (golang/go#27484), so it
// cannot complete a handshake against a CCM-8-only server to read
// ServerHello.random this way. maxVer of 0 leaves the client's default
// (highest mutually supported).
func recordAndHandshake(t *testing.T, addr string, caPEM, devicePEM, deviceKeyPEM []byte, minVer, maxVer uint16) (uint16, [32]byte) {
	t.Helper()

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		t.Fatal("failed to parse CA cert into pool")
	}
	deviceCert, err := gotls.X509KeyPair(devicePEM, deviceKeyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair (device cert): %v", err)
	}

	rawConn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("net.Dial: %v", err)
	}
	defer func() { _ = rawConn.Close() }()

	rec := &recordingConn{Conn: rawConn, buf: new(bytes.Buffer)}
	tlsConn := gotls.Client(rec, &gotls.Config{
		RootCAs: caPool,
		// gotls.Client (unlike gotls.Dial) never derives ServerName from the
		// address, and the cert covers 127.0.0.1 as a SAN.
		ServerName:   "127.0.0.1",
		Certificates: []gotls.Certificate{deviceCert},
		MinVersion:   minVer,
		MaxVersion:   maxVer,
		CipherSuites: []uint16{
			gotls.TLS_ECDHE_ECDSA_WITH_AES_128_CCM_8,
		},
		CurvePreferences: []gotls.CurveID{gotls.CurveP256},
	})
	defer func() { _ = tlsConn.Close() }()

	if err := tlsConn.Handshake(); err != nil {
		t.Fatalf("Handshake: %v", err)
	}

	return tlsConn.ConnectionState().Version, firstServerHelloRandom(t, rec.buf.Bytes())
}

// assertLegacyVersionNegotiatesTLS12 dials addr with a raw ClientHello whose
// legacy_version is legacyVersion and no supported_versions extension, and
// asserts the server answers with a TLS 1.2 ServerHello per RFC 8446 section
// 4.2.1 rather than an alert. Shared by the standard-library and CCM
// listener suites (core-go #125 amended criterion 3.4).
func assertLegacyVersionNegotiatesTLS12(t *testing.T, addr string, legacyVersion uint16) {
	t.Helper()

	hello := buildRawClientHelloNoSupportedVersions(legacyVersion)
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

// firstServerHelloRandom scans recorded raw bytes for the first Handshake
// record and returns the 32-byte random field of the ServerHello it must
// start with (the server's first handshake message).
func firstServerHelloRandom(t *testing.T, buf []byte) [32]byte {
	t.Helper()

	i := 0
	for i+5 <= len(buf) {
		contentType := buf[i]
		length := int(buf[i+3])<<8 | int(buf[i+4])
		if i+5+length > len(buf) {
			break
		}
		payload := buf[i+5 : i+5+length]
		if contentType == 22 && len(payload) >= 38 && payload[0] == 0x02 {
			var random [32]byte
			copy(random[:], payload[6:38])
			return random
		}
		i += 5 + length
	}
	t.Fatalf("no ServerHello found in %d recorded bytes", len(buf))
	return [32]byte{}
}
