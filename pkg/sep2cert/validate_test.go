package sep2cert_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2cert"
)

// validateAnchor is the fixed clock ValidateCA's time-window tests check
// against, so the tests do not depend on the wall clock at run time.
var validateAnchor = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// caTemplate returns a self-signed CA certificate template matching
// GenerateCA's shape (IsCA, BasicConstraintsValid, KeyUsageCertSign |
// KeyUsageCRLSign, no ExtKeyUsage), valid for one year around
// validateAnchor. Every rejection test below mutates exactly one field
// away from this otherwise-genuine CA, so a passing rejection test is
// never explained by some other flaw.
func caTemplate() *x509.Certificate {
	return &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test Validate CA"},
		NotBefore:             validateAnchor.Add(-time.Hour),
		NotAfter:              validateAnchor.Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
}

// selfSign generates a key on curve, signs tmpl with itself, and returns
// the parsed certificate alongside the signing key.
func selfSign(t *testing.T, tmpl *x509.Certificate, curve elliptic.Curve) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	return cert, key
}

func validateOpts() sep2cert.ValidateCAOptions {
	return sep2cert.ValidateCAOptions{Now: validateAnchor}
}

func encodeCertForTest(t *testing.T, cert *x509.Certificate) []byte {
	t.Helper()
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
}

func encodeKeyForTest(t *testing.T, key *ecdsa.PrivateKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal PKCS8 key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func TestValidateCAAcceptsGenuineCA(t *testing.T) {
	cert, key := selfSign(t, caTemplate(), elliptic.P256())
	if err := sep2cert.ValidateCA(cert, key, validateOpts()); err != nil {
		t.Fatalf("ValidateCA on a genuine CA: %v", err)
	}
}

func TestValidateCARejectsKeyMismatch(t *testing.T) {
	cert, _ := selfSign(t, caTemplate(), elliptic.P256())
	otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate other key: %v", err)
	}
	err = sep2cert.ValidateCA(cert, otherKey, validateOpts())
	if !errors.Is(err, sep2cert.ErrCAKeyMismatch) {
		t.Fatalf("ValidateCA on a mismatched pair: got %v, want ErrCAKeyMismatch", err)
	}
}

func TestValidateCARejectsNonCA(t *testing.T) {
	tmpl := caTemplate()
	tmpl.IsCA = false
	cert, key := selfSign(t, tmpl, elliptic.P256())
	err := sep2cert.ValidateCA(cert, key, validateOpts())
	if !errors.Is(err, sep2cert.ErrCANotCA) {
		t.Fatalf("ValidateCA on a leaf used as a CA: got %v, want ErrCANotCA", err)
	}
}

func TestValidateCARejectsMissingKeyUsage(t *testing.T) {
	tmpl := caTemplate()
	tmpl.KeyUsage = x509.KeyUsageDigitalSignature
	cert, key := selfSign(t, tmpl, elliptic.P256())
	err := sep2cert.ValidateCA(cert, key, validateOpts())
	if !errors.Is(err, sep2cert.ErrCAKeyUsage) {
		t.Fatalf("ValidateCA on a CA without certificate-signing usage: got %v, want ErrCAKeyUsage", err)
	}
}

func TestValidateCARejectsExtKeyUsageWithOnlyUnknownOID(t *testing.T) {
	tmpl := caTemplate()
	tmpl.UnknownExtKeyUsage = []asn1.ObjectIdentifier{{1, 3, 6, 1, 4, 1, 311, 20, 2, 1}}
	cert, key := selfSign(t, tmpl, elliptic.P256())
	err := sep2cert.ValidateCA(cert, key, validateOpts())
	if !errors.Is(err, sep2cert.ErrCAExtKeyUsage) {
		t.Fatalf("ValidateCA on a CA whose EKU holds only an unrecognized OID: got %v, want ErrCAExtKeyUsage", err)
	}
}

func TestValidateCARejectsExtKeyUsageExcludingClientAuth(t *testing.T) {
	tmpl := caTemplate()
	tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	cert, key := selfSign(t, tmpl, elliptic.P256())
	err := sep2cert.ValidateCA(cert, key, validateOpts())
	if !errors.Is(err, sep2cert.ErrCAExtKeyUsage) {
		t.Fatalf("ValidateCA on an EKU list excluding ClientAuth: got %v, want ErrCAExtKeyUsage", err)
	}
}

func TestValidateCAAcceptsExtKeyUsageIncludingClientAuth(t *testing.T) {
	tmpl := caTemplate()
	tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
	cert, key := selfSign(t, tmpl, elliptic.P256())
	if err := sep2cert.ValidateCA(cert, key, validateOpts()); err != nil {
		t.Fatalf("ValidateCA on an EKU list including ClientAuth: %v", err)
	}
}

func TestValidateCAAcceptsExtKeyUsageAny(t *testing.T) {
	tmpl := caTemplate()
	tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageAny}
	cert, key := selfSign(t, tmpl, elliptic.P256())
	if err := sep2cert.ValidateCA(cert, key, validateOpts()); err != nil {
		t.Fatalf("ValidateCA on an EKU list of anyExtendedKeyUsage: %v", err)
	}
}

func TestValidateCARejectsUnhandledCriticalExtension(t *testing.T) {
	tmpl := caTemplate()
	tmpl.ExtraExtensions = []pkix.Extension{{
		Id:       asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 99999, 7},
		Critical: true,
		Value:    []byte{0x05, 0x00}, // ASN.1 NULL
	}}
	cert, key := selfSign(t, tmpl, elliptic.P256())
	err := sep2cert.ValidateCA(cert, key, validateOpts())
	if !errors.Is(err, sep2cert.ErrCACriticalExtension) {
		t.Fatalf("ValidateCA on a CA with an unhandled critical extension: got %v, want ErrCACriticalExtension", err)
	}
}

func TestValidateCARejectsNonECDSAPublicKey(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	tmpl := caTemplate()
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &rsaKey.PublicKey, rsaKey)
	if err != nil {
		t.Fatalf("create RSA CA certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse RSA CA certificate: %v", err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ECDSA key: %v", err)
	}

	err = sep2cert.ValidateCA(cert, ecKey, validateOpts())
	if !errors.Is(err, sep2cert.ErrCAKeyType) {
		t.Fatalf("ValidateCA on an RSA CA certificate: got %v, want ErrCAKeyType", err)
	}
}

func TestValidateCARejectsCurve(t *testing.T) {
	cert, key := selfSign(t, caTemplate(), elliptic.P384())
	err := sep2cert.ValidateCA(cert, key, validateOpts())
	if !errors.Is(err, sep2cert.ErrCACurve) {
		t.Fatalf("ValidateCA on a key off P-256: got %v, want ErrCACurve", err)
	}
}

func TestValidateCARejectsNotYetValid(t *testing.T) {
	tmpl := caTemplate()
	tmpl.NotBefore = validateAnchor.Add(time.Hour)
	cert, key := selfSign(t, tmpl, elliptic.P256())
	err := sep2cert.ValidateCA(cert, key, validateOpts())
	if !errors.Is(err, sep2cert.ErrCANotYetValid) {
		t.Fatalf("ValidateCA on a not-yet-valid CA: got %v, want ErrCANotYetValid", err)
	}
}

func TestValidateCARejectsExpired(t *testing.T) {
	tmpl := caTemplate()
	tmpl.NotAfter = validateAnchor.Add(-time.Hour)
	cert, key := selfSign(t, tmpl, elliptic.P256())
	err := sep2cert.ValidateCA(cert, key, validateOpts())
	if !errors.Is(err, sep2cert.ErrCAExpired) {
		t.Fatalf("ValidateCA on an expired CA: got %v, want ErrCAExpired", err)
	}
}

// TestValidateCARejectsMinRemaining is asserted both ways in one test: the
// same certificate is refused with a MinRemaining margin set and accepted
// with it unset, so the margin check cannot be passing by coincidence.
func TestValidateCARejectsMinRemaining(t *testing.T) {
	tmpl := caTemplate()
	tmpl.NotAfter = validateAnchor.Add(2 * time.Hour)
	cert, key := selfSign(t, tmpl, elliptic.P256())

	withMargin := validateOpts()
	withMargin.MinRemaining = 24 * time.Hour
	if err := sep2cert.ValidateCA(cert, key, withMargin); !errors.Is(err, sep2cert.ErrCAExpired) {
		t.Fatalf("ValidateCA within MinRemaining of expiry: got %v, want ErrCAExpired", err)
	}

	if err := sep2cert.ValidateCA(cert, key, validateOpts()); err != nil {
		t.Fatalf("ValidateCA with MinRemaining unset, same certificate: %v", err)
	}
}

// TestValidateCAAcceptsIntermediateByDefault is asserted both ways: the
// same intermediate CA passes with RequireSelfSigned off and is refused
// with it on, so the option is proven to change the outcome rather than
// being ignored.
func TestValidateCAAcceptsIntermediateByDefault(t *testing.T) {
	rootTmpl := caTemplate()
	rootTmpl.SerialNumber = big.NewInt(2)
	rootTmpl.Subject = pkix.Name{CommonName: "Test Root"}
	rootCert, rootKey := selfSign(t, rootTmpl, elliptic.P256())

	interKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate intermediate key: %v", err)
	}
	interTmpl := caTemplate()
	interTmpl.SerialNumber = big.NewInt(3)
	interTmpl.Subject = pkix.Name{CommonName: "Test Intermediate"}
	der, err := x509.CreateCertificate(rand.Reader, interTmpl, rootCert, &interKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("create intermediate certificate: %v", err)
	}
	interCert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse intermediate certificate: %v", err)
	}

	if err := sep2cert.ValidateCA(interCert, interKey, validateOpts()); err != nil {
		t.Fatalf("ValidateCA on an intermediate CA, RequireSelfSigned off: %v", err)
	}

	strict := validateOpts()
	strict.RequireSelfSigned = true
	err = sep2cert.ValidateCA(interCert, interKey, strict)
	if !errors.Is(err, sep2cert.ErrCANotSelfSigned) {
		t.Fatalf("ValidateCA on an intermediate CA, RequireSelfSigned on: got %v, want ErrCANotSelfSigned", err)
	}
}

// TestValidateCAErrorNamesCertificateNotKeyMaterial covers item 1 of the
// brief: a refusal names which certificate failed and never prints key
// material.
func TestValidateCAErrorNamesCertificateNotKeyMaterial(t *testing.T) {
	tmpl := caTemplate()
	tmpl.Subject = pkix.Name{CommonName: "Identify Me"}
	tmpl.IsCA = false
	cert, key := selfSign(t, tmpl, elliptic.P256())

	err := sep2cert.ValidateCA(cert, key, validateOpts())
	if err == nil {
		t.Fatal("want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Identify Me") {
		t.Errorf("error %q does not name the certificate", msg)
	}

	keyDER, marshalErr := x509.MarshalECPrivateKey(key)
	if marshalErr != nil {
		t.Fatalf("marshal key: %v", marshalErr)
	}
	if strings.Contains(msg, string(keyDER)) {
		t.Errorf("error %q contains raw key material", msg)
	}
}

func TestParseCAPairAcceptsGenuinePair(t *testing.T) {
	cert, key := selfSign(t, caTemplate(), elliptic.P256())
	certPEM := encodeCertForTest(t, cert)
	keyPEM := encodeKeyForTest(t, key)

	gotCert, gotKey, err := sep2cert.ParseCAPair(certPEM, keyPEM, validateOpts())
	if err != nil {
		t.Fatalf("ParseCAPair on a genuine pair: %v", err)
	}
	if gotCert.SerialNumber.Cmp(cert.SerialNumber) != 0 {
		t.Errorf("ParseCAPair returned the wrong certificate")
	}
	if !gotKey.Equal(key) {
		t.Errorf("ParseCAPair returned the wrong key")
	}
}

func TestParseCAPairRejectsMismatchedPair(t *testing.T) {
	cert, _ := selfSign(t, caTemplate(), elliptic.P256())
	otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate other key: %v", err)
	}
	certPEM := encodeCertForTest(t, cert)
	keyPEM := encodeKeyForTest(t, otherKey)

	_, _, err = sep2cert.ParseCAPair(certPEM, keyPEM, validateOpts())
	if !errors.Is(err, sep2cert.ErrCAKeyMismatch) {
		t.Fatalf("ParseCAPair on a mismatched pair: got %v, want ErrCAKeyMismatch", err)
	}
}

func TestParseCAPairRejectsExpired(t *testing.T) {
	tmpl := caTemplate()
	tmpl.NotAfter = validateAnchor.Add(-time.Hour)
	cert, key := selfSign(t, tmpl, elliptic.P256())
	certPEM := encodeCertForTest(t, cert)
	keyPEM := encodeKeyForTest(t, key)

	_, _, err := sep2cert.ParseCAPair(certPEM, keyPEM, validateOpts())
	if !errors.Is(err, sep2cert.ErrCAExpired) {
		t.Fatalf("ParseCAPair on an expired CA: got %v, want ErrCAExpired", err)
	}
}

// TestLoadCAAcceptsGenuineCA and TestLoadCARefusesNonCA are asserted both
// ways: the same file-loading path accepts a genuine CA and refuses a
// non-CA, so the wiring from LoadCA into ParseCAPair is proven, not
// assumed. Every temp file lives under t.TempDir().
func TestLoadCAAcceptsGenuineCA(t *testing.T) {
	dir := t.TempDir()
	cert, key := selfSign(t, caTemplate(), elliptic.P256())
	certFile := filepath.Join(dir, "ca.pem")
	keyFile := filepath.Join(dir, "ca-key.pem")
	if err := os.WriteFile(certFile, encodeCertForTest(t, cert), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, encodeKeyForTest(t, key), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	gotCert, gotKey, err := sep2cert.LoadCA(certFile, keyFile, validateOpts())
	if err != nil {
		t.Fatalf("LoadCA on a genuine CA: %v", err)
	}
	if gotCert.SerialNumber.Cmp(cert.SerialNumber) != 0 {
		t.Errorf("LoadCA returned the wrong certificate")
	}
	if !gotKey.Equal(key) {
		t.Errorf("LoadCA returned the wrong key")
	}
}

func TestLoadCARefusesNonCA(t *testing.T) {
	dir := t.TempDir()
	tmpl := caTemplate()
	tmpl.IsCA = false
	cert, key := selfSign(t, tmpl, elliptic.P256())
	certFile := filepath.Join(dir, "ca.pem")
	keyFile := filepath.Join(dir, "ca-key.pem")
	if err := os.WriteFile(certFile, encodeCertForTest(t, cert), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, encodeKeyForTest(t, key), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	_, _, err := sep2cert.LoadCA(certFile, keyFile, validateOpts())
	if !errors.Is(err, sep2cert.ErrCANotCA) {
		t.Fatalf("LoadCA on a non-CA cert file: got %v, want ErrCANotCA", err)
	}
	if !strings.Contains(err.Error(), certFile) {
		t.Errorf("LoadCA error %q does not name the file %q", err, certFile)
	}
}
