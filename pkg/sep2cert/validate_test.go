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

func TestValidateCARejectsNilCertificate(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	err = sep2cert.ValidateCA(nil, key, validateOpts())
	if err == nil {
		t.Fatal("ValidateCA with a nil certificate: want error, got nil")
	}
	if !strings.Contains(err.Error(), "certificate is nil") {
		t.Errorf("ValidateCA with a nil certificate: got %q, want it to name the nil certificate", err)
	}
}

func TestValidateCARejectsNilKey(t *testing.T) {
	cert, _ := selfSign(t, caTemplate(), elliptic.P256())
	err := sep2cert.ValidateCA(cert, nil, validateOpts())
	if err == nil {
		t.Fatal("ValidateCA with a nil key: want error, got nil")
	}
	if !strings.Contains(err.Error(), "private key is nil") {
		t.Errorf("ValidateCA with a nil key: got %q, want it to name the nil key", err)
	}
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

// TestValidateCARejectsBasicConstraintsInvalidWithIsCATrue covers the
// half of the IsCA/BasicConstraintsValid check a parsed certificate can
// never separate: ValidateCA takes a *x509.Certificate directly, so a
// hand-built value can set IsCA true while leaving BasicConstraintsValid
// false, which no certificate produced by CreateCertificate or
// ParseCertificate can do.
func TestValidateCARejectsBasicConstraintsInvalidWithIsCATrue(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	cert := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Hand-built Non-CA"},
		NotBefore:             validateAnchor.Add(-time.Hour),
		NotAfter:              validateAnchor.Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		PublicKey:             &key.PublicKey,
		IsCA:                  true,
		BasicConstraintsValid: false,
	}

	err = sep2cert.ValidateCA(cert, key, validateOpts())
	if !errors.Is(err, sep2cert.ErrCANotCA) {
		t.Fatalf("ValidateCA with IsCA true but BasicConstraintsValid false: got %v, want ErrCANotCA", err)
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

// TestValidateCACriticalExtensionNamesTheOID checks that the refusal
// carries the offending OID, since cert.UnhandledCriticalExtensions holds
// it right where the refusal is made and the operator otherwise gets only
// a category, not something to go look at.
func TestValidateCACriticalExtensionNamesTheOID(t *testing.T) {
	oid := asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 99999, 7}
	tmpl := caTemplate()
	tmpl.ExtraExtensions = []pkix.Extension{{
		Id:       oid,
		Critical: true,
		Value:    []byte{0x05, 0x00}, // ASN.1 NULL
	}}
	cert, key := selfSign(t, tmpl, elliptic.P256())
	err := sep2cert.ValidateCA(cert, key, validateOpts())
	if !errors.Is(err, sep2cert.ErrCACriticalExtension) {
		t.Fatalf("ValidateCA on a CA with an unhandled critical extension: got %v, want ErrCACriticalExtension", err)
	}
	if !strings.Contains(err.Error(), oid.String()) {
		t.Errorf("ValidateCA error %q does not name the offending OID %s", err, oid.String())
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
	err := sep2cert.ValidateCA(cert, key, withMargin)
	if !errors.Is(err, sep2cert.ErrCAExpiringSoon) {
		t.Fatalf("ValidateCA within MinRemaining of expiry: got %v, want ErrCAExpiringSoon", err)
	}
	if strings.Contains(err.Error(), "has expired") {
		t.Errorf("ValidateCA within MinRemaining of expiry claims the CA has expired, but it has not: %v", err)
	}

	if err := sep2cert.ValidateCA(cert, key, validateOpts()); err != nil {
		t.Fatalf("ValidateCA with MinRemaining unset, same certificate: %v", err)
	}
}

// TestValidateCAAcceptsAtNotBefore and TestValidateCARejectsOneSecondBeforeNotBefore
// pin the NotBefore boundary as inclusive: an implementation elsewhere on the
// wire assumes a CA is usable at the exact instant its window opens. The
// offset is one second, not one nanosecond: ASN.1 GeneralizedTime has
// one-second resolution, so x509.CreateCertificate floors NotBefore/NotAfter
// to the second, and a sub-second offset would silently round away.
func TestValidateCAAcceptsAtNotBefore(t *testing.T) {
	tmpl := caTemplate()
	tmpl.NotBefore = validateAnchor
	cert, key := selfSign(t, tmpl, elliptic.P256())
	if err := sep2cert.ValidateCA(cert, key, validateOpts()); err != nil {
		t.Fatalf("ValidateCA at exactly NotBefore: %v", err)
	}
}

func TestValidateCARejectsOneSecondBeforeNotBefore(t *testing.T) {
	tmpl := caTemplate()
	tmpl.NotBefore = validateAnchor.Add(time.Second)
	cert, key := selfSign(t, tmpl, elliptic.P256())
	err := sep2cert.ValidateCA(cert, key, validateOpts())
	if !errors.Is(err, sep2cert.ErrCANotYetValid) {
		t.Fatalf("ValidateCA one second before NotBefore: got %v, want ErrCANotYetValid", err)
	}
}

// TestValidateCAAcceptsAtNotAfter and TestValidateCARejectsOneSecondAfterNotAfter
// pin the NotAfter boundary as inclusive, the same as NotBefore, at the same
// one-second resolution.
func TestValidateCAAcceptsAtNotAfter(t *testing.T) {
	tmpl := caTemplate()
	tmpl.NotAfter = validateAnchor
	cert, key := selfSign(t, tmpl, elliptic.P256())
	if err := sep2cert.ValidateCA(cert, key, validateOpts()); err != nil {
		t.Fatalf("ValidateCA at exactly NotAfter: %v", err)
	}
}

func TestValidateCARejectsOneSecondAfterNotAfter(t *testing.T) {
	tmpl := caTemplate()
	tmpl.NotAfter = validateAnchor.Add(-time.Second)
	cert, key := selfSign(t, tmpl, elliptic.P256())
	err := sep2cert.ValidateCA(cert, key, validateOpts())
	if !errors.Is(err, sep2cert.ErrCAExpired) {
		t.Fatalf("ValidateCA one second after NotAfter: got %v, want ErrCAExpired", err)
	}
}

// TestValidateCAAcceptsAtExactMinRemaining and
// TestValidateCARejectsOneSecondInsideMinRemaining pin the MinRemaining
// boundary: exactly the margin remaining is accepted, one second less is
// refused. MinRemaining is compared against a duration derived from
// NotAfter, which is itself second-resolution once round-tripped through
// the certificate encoding, so this boundary is also one second wide.
func TestValidateCAAcceptsAtExactMinRemaining(t *testing.T) {
	tmpl := caTemplate()
	tmpl.NotAfter = validateAnchor.Add(24 * time.Hour)
	cert, key := selfSign(t, tmpl, elliptic.P256())
	opts := validateOpts()
	opts.MinRemaining = 24 * time.Hour
	if err := sep2cert.ValidateCA(cert, key, opts); err != nil {
		t.Fatalf("ValidateCA with remaining validity exactly equal to MinRemaining: %v", err)
	}
}

func TestValidateCARejectsOneSecondInsideMinRemaining(t *testing.T) {
	tmpl := caTemplate()
	tmpl.NotAfter = validateAnchor.Add(24*time.Hour - time.Second)
	cert, key := selfSign(t, tmpl, elliptic.P256())
	opts := validateOpts()
	opts.MinRemaining = 24 * time.Hour
	err := sep2cert.ValidateCA(cert, key, opts)
	if !errors.Is(err, sep2cert.ErrCAExpiringSoon) {
		t.Fatalf("ValidateCA one second inside MinRemaining: got %v, want ErrCAExpiringSoon", err)
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

func TestParseCAPairRejectsMalformedCertPEM(t *testing.T) {
	_, key := selfSign(t, caTemplate(), elliptic.P256())
	keyPEM := encodeKeyForTest(t, key)

	_, _, err := sep2cert.ParseCAPair([]byte("not PEM data"), keyPEM, validateOpts())
	if err == nil {
		t.Fatal("ParseCAPair with malformed certificate PEM: want error, got nil")
	}
	if !strings.Contains(err.Error(), "parse CA cert") {
		t.Errorf("ParseCAPair with malformed certificate PEM: got %q, want it to name the certificate file", err)
	}
}

func TestParseCAPairRejectsMalformedKeyPEM(t *testing.T) {
	cert, _ := selfSign(t, caTemplate(), elliptic.P256())
	certPEM := encodeCertForTest(t, cert)

	_, _, err := sep2cert.ParseCAPair(certPEM, []byte("not PEM data"), validateOpts())
	if err == nil {
		t.Fatal("ParseCAPair with malformed key PEM: want error, got nil")
	}
	if !strings.Contains(err.Error(), "parse CA key") {
		t.Errorf("ParseCAPair with malformed key PEM: got %q, want it to name the key file", err)
	}
}

// TestParseCAPairAcceptsGenerateCAOutput is the regression test for the
// generator-to-validator round trip: caTemplate is a hand-written copy of
// GenerateCA's shape, and nothing else in this file proves the copy is
// accurate. GenerateCA also attaches a critical anyPolicy extension that
// caTemplate omits, so this exercises a path caTemplate cannot.
func TestParseCAPairAcceptsGenerateCAOutput(t *testing.T) {
	certPEM, keyPEM, err := sep2cert.GenerateCA(sep2cert.CAOptions{
		Organization: "Test Org",
		CommonName:   "Generated Test CA",
		ValidYears:   1,
	})
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}

	if _, _, err := sep2cert.ParseCAPair(certPEM, keyPEM, sep2cert.ValidateCAOptions{}); err != nil {
		t.Fatalf("ParseCAPair on GenerateCA's own output: %v", err)
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

// TestLoadCAAppliesExplicitOptions proves LoadCA's variadic option reaches
// ParseCAPair rather than being dropped. It cannot pass by wall-clock
// coincidence: the same certificate loads with no opts and is refused only
// when an explicit MinRemaining option is supplied, so the option itself is
// what has to change the outcome.
func TestLoadCAAppliesExplicitOptions(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	tmpl := caTemplate()
	tmpl.NotBefore = now.Add(-time.Hour)
	tmpl.NotAfter = now.Add(48 * time.Hour)
	cert, key := selfSign(t, tmpl, elliptic.P256())
	certFile := filepath.Join(dir, "ca.pem")
	keyFile := filepath.Join(dir, "ca-key.pem")
	if err := os.WriteFile(certFile, encodeCertForTest(t, cert), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, encodeKeyForTest(t, key), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	if _, _, err := sep2cert.LoadCA(certFile, keyFile); err != nil {
		t.Fatalf("LoadCA with no opts on a CA valid for 48h: %v", err)
	}

	opt := sep2cert.ValidateCAOptions{MinRemaining: 72 * time.Hour}
	_, _, err := sep2cert.LoadCA(certFile, keyFile, opt)
	if !errors.Is(err, sep2cert.ErrCAExpiringSoon) {
		t.Fatalf("LoadCA with an explicit MinRemaining option: got %v, want ErrCAExpiringSoon", err)
	}
}

// TestLoadCARefusesMultipleOptions is asserted both ways: a single option
// value still applies (proven by TestLoadCAAppliesExplicitOptions above),
// and a second value refuses with ErrLoadCATooManyOptions instead of the
// first being applied and the second silently dropped.
func TestLoadCARefusesMultipleOptions(t *testing.T) {
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

	_, _, err := sep2cert.LoadCA(certFile, keyFile, validateOpts(), validateOpts())
	if !errors.Is(err, sep2cert.ErrLoadCATooManyOptions) {
		t.Fatalf("LoadCA with two option values: got %v, want ErrLoadCATooManyOptions", err)
	}
}

// TestLoadCANamesWhichFileFailedToRead covers LoadCA's two file-read
// error paths separately, so a swap of the two messages (a missing cert
// file reporting "read CA key" or vice versa) fails: the message is what
// tells an operator which file to look at.
func TestLoadCANamesWhichFileFailedToRead(t *testing.T) {
	dir := t.TempDir()
	cert, key := selfSign(t, caTemplate(), elliptic.P256())

	t.Run("missing cert file", func(t *testing.T) {
		certFile := filepath.Join(dir, "missing-ca.pem")
		keyFile := filepath.Join(dir, "present-ca-key.pem")
		if err := os.WriteFile(keyFile, encodeKeyForTest(t, key), 0o600); err != nil {
			t.Fatalf("write key: %v", err)
		}
		_, _, err := sep2cert.LoadCA(certFile, keyFile, validateOpts())
		if err == nil {
			t.Fatal("LoadCA with a missing cert file: want error, got nil")
		}
		if !strings.Contains(err.Error(), "read CA cert") {
			t.Errorf("LoadCA with a missing cert file: got %q, want it to name the cert read", err)
		}
		if strings.Contains(err.Error(), "read CA key") {
			t.Errorf("LoadCA with a missing cert file: got %q, names the key read instead", err)
		}
	})

	t.Run("missing key file", func(t *testing.T) {
		certFile := filepath.Join(dir, "present-ca.pem")
		keyFile := filepath.Join(dir, "missing-ca-key.pem")
		if err := os.WriteFile(certFile, encodeCertForTest(t, cert), 0o600); err != nil {
			t.Fatalf("write cert: %v", err)
		}
		_, _, err := sep2cert.LoadCA(certFile, keyFile, validateOpts())
		if err == nil {
			t.Fatal("LoadCA with a missing key file: want error, got nil")
		}
		if !strings.Contains(err.Error(), "read CA key") {
			t.Errorf("LoadCA with a missing key file: got %q, want it to name the key read", err)
		}
		if strings.Contains(err.Error(), "read CA cert") {
			t.Errorf("LoadCA with a missing key file: got %q, names the cert read instead", err)
		}
	})
}

// TestLoadCADefaultAllowsIntermediate and TestLoadCADefaultAppliesNoMinRemainingMargin
// pin the two policy defaults pem.go's LoadCA pins by calling it with no
// opts at all, the exact zero-value path both consumers reach. They use the
// real wall clock, not validateAnchor, because the point is the behavior of
// ValidateCAOptions{}'s zero Now, not a fixed one.
func TestLoadCADefaultAllowsIntermediate(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	rootTmpl := caTemplate()
	rootTmpl.SerialNumber = big.NewInt(10)
	rootTmpl.Subject = pkix.Name{CommonName: "Default Test Root"}
	rootTmpl.NotBefore = now.Add(-time.Hour)
	rootTmpl.NotAfter = now.Add(24 * time.Hour)
	rootCert, rootKey := selfSign(t, rootTmpl, elliptic.P256())

	interKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate intermediate key: %v", err)
	}
	interTmpl := caTemplate()
	interTmpl.SerialNumber = big.NewInt(11)
	interTmpl.Subject = pkix.Name{CommonName: "Default Test Intermediate"}
	interTmpl.NotBefore = now.Add(-time.Hour)
	interTmpl.NotAfter = now.Add(24 * time.Hour)
	der, err := x509.CreateCertificate(rand.Reader, interTmpl, rootCert, &interKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("create intermediate certificate: %v", err)
	}
	interCert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse intermediate certificate: %v", err)
	}

	certFile := filepath.Join(dir, "ca.pem")
	keyFile := filepath.Join(dir, "ca-key.pem")
	if err := os.WriteFile(certFile, encodeCertForTest(t, interCert), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, encodeKeyForTest(t, interKey), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	if _, _, err := sep2cert.LoadCA(certFile, keyFile); err != nil {
		t.Fatalf("LoadCA on an intermediate CA with no opts: %v", err)
	}
}

func TestLoadCADefaultAppliesNoMinRemainingMargin(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	tmpl := caTemplate()
	tmpl.NotBefore = now.Add(-time.Hour)
	tmpl.NotAfter = now.Add(10 * 24 * time.Hour) // 10 days remaining: under the 30-day mutant, over zero
	cert, key := selfSign(t, tmpl, elliptic.P256())
	certFile := filepath.Join(dir, "ca.pem")
	keyFile := filepath.Join(dir, "ca-key.pem")
	if err := os.WriteFile(certFile, encodeCertForTest(t, cert), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, encodeKeyForTest(t, key), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	if _, _, err := sep2cert.LoadCA(certFile, keyFile); err != nil {
		t.Fatalf("LoadCA on a CA 10 days from expiry with no opts: %v", err)
	}
}
