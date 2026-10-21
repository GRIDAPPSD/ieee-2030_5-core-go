package sep2cert_test

import (
	"bytes"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2cert"
)

// hmnSANContent returns the DER of one HMN otherName GeneralName, the bytes
// that sit inside the outer GeneralNames SEQUENCE.
func hmnSANContent(t *testing.T, hwType asn1.ObjectIdentifier, serial []byte) []byte {
	t.Helper()
	hmnDER, err := asn1.Marshal(struct {
		HWType      asn1.ObjectIdentifier
		HWSerialNum asn1.RawValue
	}{hwType, asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagOctetString, Bytes: serial}})
	if err != nil {
		t.Fatalf("marshal HMN: %v", err)
	}
	on, err := asn1.MarshalWithParams(struct {
		TypeID asn1.ObjectIdentifier
		Value  asn1.RawValue
	}{
		sep2cert.OIDHardwareModuleName,
		asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: hmnDER},
	}, "tag:0")
	if err != nil {
		t.Fatalf("marshal otherName: %v", err)
	}
	return on
}

func certWithSAN(content []byte, outer asn1.RawValue) *x509.Certificate {
	outer.Bytes = content
	der, err := asn1.Marshal(outer)
	if err != nil {
		panic(err)
	}
	return &x509.Certificate{Extensions: []pkix.Extension{{Id: sep2cert.OIDSubjectAltName, Value: der}}}
}

var seqOuter = asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSequence, IsCompound: true}

func TestHMNParsersAgreeOnValidSAN(t *testing.T) {
	hwType := asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 40732, 9}
	serial := []byte{0xde, 0xad, 0xbe, 0xef}
	cert := certWithSAN(hmnSANContent(t, hwType, serial), seqOuter)

	got, ok := sep2cert.ExtractHardwareModuleName(cert)
	if !ok {
		t.Fatal("ExtractHardwareModuleName refused a valid SAN")
	}
	if !got.HWType.Equal(hwType) || !bytes.Equal(got.HWSerialNum, serial) {
		t.Errorf("got type %v serial %x, want %v %x", got.HWType, got.HWSerialNum, hwType, serial)
	}
	all, present := sep2cert.SANAllOtherNamesAreHardwareModuleName(cert)
	if !all || !present {
		t.Errorf("strict = (%v, %v), want (true, true)", all, present)
	}
}

func TestHMNParsersRefuseMalformedOuterSequence(t *testing.T) {
	content := hmnSANContent(t, asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 40732, 9}, []byte{1, 2, 3})
	cases := []struct {
		name  string
		outer asn1.RawValue
	}{
		{"SET instead of SEQUENCE", asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSet, IsCompound: true}},
		{"primitive SEQUENCE tag", asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSequence, IsCompound: false}},
		{"context-specific outer", asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: asn1.TagSequence, IsCompound: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cert := certWithSAN(content, tc.outer)
			if got, ok := sep2cert.ExtractHardwareModuleName(cert); ok {
				t.Errorf("ExtractHardwareModuleName accepted malformed outer: %+v", got)
			}
			all, present := sep2cert.SANAllOtherNamesAreHardwareModuleName(cert)
			if all || !present {
				t.Errorf("strict = (%v, %v), want (false, true)", all, present)
			}
		})
	}
}
