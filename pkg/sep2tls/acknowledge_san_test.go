package sep2tls

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2cert"
)

// sanWithDNSName appends a dNSName GeneralName to a SAN built from the given
// otherName specs.
func sanWithDNSName(t *testing.T, specs []otherNameSpec, dns string) pkix.Extension {
	t.Helper()
	ext := mustBuildMultiOtherNameSAN(t, specs)
	var seq asn1.RawValue
	if _, err := asn1.Unmarshal(ext.Value, &seq); err != nil {
		t.Fatalf("unmarshal SAN: %v", err)
	}
	dnsName := append([]byte{0x82, byte(len(dns))}, dns...)
	val, err := asn1.Marshal(asn1.RawValue{
		Class:      asn1.ClassUniversal,
		Tag:        asn1.TagSequence,
		IsCompound: true,
		Bytes:      append(append([]byte{}, seq.Bytes...), dnsName...),
	})
	if err != nil {
		t.Fatalf("marshal SAN: %v", err)
	}
	ext.Value = val
	return ext
}

func hasOID(oids []asn1.ObjectIdentifier, want asn1.ObjectIdentifier) bool {
	for _, o := range oids {
		if o.Equal(want) {
			return true
		}
	}
	return false
}

func TestAcknowledgeHardwareModuleSAN(t *testing.T) {
	caCert, caKey := genCA(t)
	hmn := otherNameSpec{OID: sep2cert.OIDHardwareModuleName, InnerKind: innerKindValidHMN}
	unknownCritical := pkix.Extension{Id: asn1.ObjectIdentifier{1, 2, 3, 4, 5, 6, 7}, Critical: true, Value: []byte{0x05, 0x00}}

	tests := []struct {
		name         string
		extra        []pkix.Extension
		stdlibSkips  bool // stdlib parses the DNS name, so the SAN never reaches the unhandled list
		wantSANKept  bool
		wantUnknownK bool
	}{
		{"HMN only is cleared", []pkix.Extension{mustBuildMultiOtherNameSAN(t, []otherNameSpec{hmn})}, false, false, false},
		{"HMN with DNS name is never unhandled", []pkix.Extension{sanWithDNSName(t, []otherNameSpec{hmn}, "a.example")}, true, false, false},
		{"foreign otherName OID is kept", []pkix.Extension{mustBuildMultiOtherNameSAN(t, []otherNameSpec{{OID: asn1.ObjectIdentifier{1, 2, 3, 4}, InnerKind: innerKindValidHMN}})}, false, true, false},
		{"HMN beside a foreign otherName is kept", []pkix.Extension{mustBuildMultiOtherNameSAN(t, []otherNameSpec{hmn, {OID: asn1.ObjectIdentifier{1, 2, 3, 4}, InnerKind: innerKindArbitraryBytes}})}, false, true, false},
		{"malformed HMN inner bytes are kept", []pkix.Extension{mustBuildMalformedHMNSAN(t)}, false, true, false},
		{"empty SAN is kept", []pkix.Extension{mustBuildMultiOtherNameSAN(t, nil)}, false, true, false},
		{"HMN SAN cleared but other unknown critical extension kept", []pkix.Extension{mustBuildMultiOtherNameSAN(t, []otherNameSpec{hmn}), unknownCritical}, false, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			der := mintRawCertDER(t, caCert, caKey, tc.extra)
			cert, err := x509.ParseCertificate(der)
			if err != nil {
				t.Fatalf("ParseCertificate: %v", err)
			}
			if !tc.stdlibSkips && !hasOID(cert.UnhandledCriticalExtensions, sep2cert.OIDSubjectAltName) {
				t.Fatalf("precondition: SAN OID not unhandled before call: %v", cert.UnhandledCriticalExtensions)
			}
			AcknowledgeHardwareModuleSAN(cert)
			if tc.stdlibSkips {
				if allHMN, _ := sep2cert.SANAllOtherNamesAreHardwareModuleName(cert); allHMN {
					t.Error("strict checker accepted a SAN carrying a dNSName")
				}
			}
			if got := hasOID(cert.UnhandledCriticalExtensions, sep2cert.OIDSubjectAltName); got != tc.wantSANKept {
				t.Errorf("SAN OID still unhandled = %v, want %v (%v)", got, tc.wantSANKept, cert.UnhandledCriticalExtensions)
			}
			if got := hasOID(cert.UnhandledCriticalExtensions, unknownCritical.Id); got != tc.wantUnknownK {
				t.Errorf("unknown critical OID still unhandled = %v, want %v", got, tc.wantUnknownK)
			}
		})
	}
}

// A ServerAuth-only leaf cannot pass VerifyPeerCertWithHardwareModuleSAN
// (it pins ClientAuth), but the exported rule lets its owner verify it with
// any EKU.
func TestAcknowledgeHardwareModuleSANServerAuthLeaf(t *testing.T) {
	caCert, caKey := genCA(t)
	san := mustBuildMultiOtherNameSAN(t, []otherNameSpec{{OID: sep2cert.OIDHardwareModuleName, InnerKind: innerKindValidHMN}})
	leaf, err := x509.ParseCertificate(mintRawCertDERWithEKU(t, caCert, caKey, []pkix.Extension{san}, x509.ExtKeyUsageServerAuth))
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	if err := VerifyPeerCertWithHardwareModuleSAN([][]byte{leaf.Raw}, roots); err == nil {
		t.Fatal("ClientAuth-pinned verifier accepted a ServerAuth-only leaf; behavior changed")
	}

	opts := x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	if _, err := leaf.Verify(opts); err == nil {
		t.Fatal("precondition: Verify passed without acknowledging the critical SAN")
	}
	AcknowledgeHardwareModuleSAN(leaf)
	if _, err := leaf.Verify(opts); err != nil {
		t.Fatalf("Verify after AcknowledgeHardwareModuleSAN: %v", err)
	}
}

// Both verifier outcomes for the same HMN-only leaf: accepted with the
// ClientAuth EKU it pins, rejected when a foreign otherName rides along.
func TestVerifyPeerCertWithHardwareModuleSANUsesRule(t *testing.T) {
	caCert, caKey := genCA(t)
	roots := x509.NewCertPool()
	roots.AddCert(caCert)
	hmn := otherNameSpec{OID: sep2cert.OIDHardwareModuleName, InnerKind: innerKindValidHMN}

	good := mintRawCertDER(t, caCert, caKey, []pkix.Extension{mustBuildMultiOtherNameSAN(t, []otherNameSpec{hmn})})
	if err := VerifyPeerCertWithHardwareModuleSAN([][]byte{good}, roots); err != nil {
		t.Errorf("HMN-only ClientAuth leaf rejected: %v", err)
	}
	bad := mintRawCertDER(t, caCert, caKey, []pkix.Extension{mustBuildMultiOtherNameSAN(t, []otherNameSpec{hmn, {OID: asn1.ObjectIdentifier{1, 2, 3, 4}, InnerKind: innerKindArbitraryBytes}})})
	if err := VerifyPeerCertWithHardwareModuleSAN([][]byte{bad}, roots); err == nil {
		t.Error("HMN beside a foreign otherName accepted; want fail-closed")
	}
}
