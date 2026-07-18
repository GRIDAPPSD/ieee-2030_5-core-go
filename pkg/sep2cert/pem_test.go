package sep2cert_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2cert"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls"
)

// TestCertificateDERMatchesPEMBlockBytes verifies CertificateDER returns
// exactly the DER bytes carried in the PEM block, and that the SHA-256 of
// those bytes matches the SHA-256 of the block's own Bytes field. This is
// the round-trip correctness check: CertificateDER must not repackage,
// truncate, or otherwise alter the DER payload.
func TestCertificateDERMatchesPEMBlockBytes(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	certPEM, _, err := sep2cert.GenerateDeviceCert(caCert, caKey, sep2cert.DeviceCertOptions{
		DeviceType:  sep2cert.DeviceTypeGeneric,
		HWSerialNum: "DER-ROUNDTRIP-001",
	})
	if err != nil {
		t.Fatalf("GenerateDeviceCert: %v", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("failed to decode PEM cert")
	}

	der, err := sep2cert.CertificateDER(certPEM)
	if err != nil {
		t.Fatalf("CertificateDER: %v", err)
	}

	if string(der) != string(block.Bytes) {
		t.Errorf("CertificateDER returned %d bytes, block.Bytes is %d bytes: not equal", len(der), len(block.Bytes))
	}

	gotSum := sha256.Sum256(der)
	wantSum := sha256.Sum256(block.Bytes)
	if gotSum != wantSum {
		t.Errorf("sha256(CertificateDER(certPEM)) = %x, want %x", gotSum, wantSum)
	}
}

// TestCertificateDERMatchesFingerprintAndLFDI is the identity invariant
// check: sha256(CertificateDER(certPEM)) must equal sep2tls.Fingerprint(cert),
// and the hand-computed LFDI from CertificateDER's hash (first 20 bytes,
// hex-encoded uppercase) must equal sep2tls.LFDI(cert) exactly. This is
// the invariant the doc comment on CertificateDER promises.
func TestCertificateDERMatchesFingerprintAndLFDI(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	certPEM, _, err := sep2cert.GenerateDeviceCert(caCert, caKey, sep2cert.DeviceCertOptions{
		DeviceType:  sep2cert.DeviceTypeGeneric,
		HWSerialNum: "DER-IDENTITY-001",
	})
	if err != nil {
		t.Fatalf("GenerateDeviceCert: %v", err)
	}

	cert, err := sep2cert.ParseCertificatePEM(certPEM)
	if err != nil {
		t.Fatalf("ParseCertificatePEM: %v", err)
	}

	der, err := sep2cert.CertificateDER(certPEM)
	if err != nil {
		t.Fatalf("CertificateDER: %v", err)
	}

	gotFingerprint := sha256.Sum256(der)
	wantFingerprint := sep2tls.Fingerprint(cert)
	if gotFingerprint != wantFingerprint {
		t.Errorf("sha256(CertificateDER(certPEM)) = %x, want sep2tls.Fingerprint(cert) = %x", gotFingerprint, wantFingerprint)
	}

	gotLFDI := strings.ToUpper(hex.EncodeToString(gotFingerprint[:20]))
	wantLFDI := sep2tls.LFDI(cert)
	if gotLFDI != wantLFDI {
		t.Errorf("LFDI computed from CertificateDER = %q, want sep2tls.LFDI(cert) = %q", gotLFDI, wantLFDI)
	}
}

// TestCertificateDERRejectsWrongBlockType verifies CertificateDER fails
// closed on a PEM block that is not of type CERTIFICATE, rather than
// silently returning the wrong block's bytes.
func TestCertificateDERRejectsWrongBlockType(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	_, keyPEM, err := sep2cert.GenerateDeviceCert(caCert, caKey, sep2cert.DeviceCertOptions{
		DeviceType:  sep2cert.DeviceTypeGeneric,
		HWSerialNum: "DER-WRONGTYPE-001",
	})
	if err != nil {
		t.Fatalf("GenerateDeviceCert: %v", err)
	}

	// keyPEM decodes to a PEM block, but its type is a private key, not
	// a certificate.
	if _, err := sep2cert.CertificateDER(keyPEM); err == nil {
		t.Fatal("CertificateDER on a PRIVATE KEY block: want error, got nil")
	}

	ecParamsPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PARAMETERS",
		Bytes: []byte{0x06, 0x08, 0x2a, 0x86, 0x48, 0xce, 0x3d, 0x03, 0x01, 0x07},
	})
	if _, err := sep2cert.CertificateDER(ecParamsPEM); err == nil {
		t.Fatal("CertificateDER on an EC PARAMETERS block: want error, got nil")
	}
}

// TestCertificateDERRejectsUnparsablePEM verifies CertificateDER fails
// closed on input that does not decode as PEM at all, rather than
// returning a zero-value or partial result.
func TestCertificateDERRejectsUnparsablePEM(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
	}{
		{"empty input", []byte{}},
		{"garbage bytes", []byte("this is not PEM data at all")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			der, err := sep2cert.CertificateDER(tc.in)
			if err == nil {
				t.Fatalf("CertificateDER(%q): want error, got nil (der=%v)", tc.in, der)
			}
			if der != nil {
				t.Errorf("CertificateDER(%q): want nil bytes on error, got %v", tc.in, der)
			}
		})
	}
}
