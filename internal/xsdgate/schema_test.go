package xsdgate_test

import (
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/internal/xsdgate"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/schema"
)

// TestVendoredSchemaParses is the load-bearing guard on the whole gate. If
// the vendored schema ever gains a construct outside the supported subset,
// ParseSchema errors here rather than letting the gate silently check less
// than its documentation claims.
func TestVendoredSchemaParses(t *testing.T) {
	s, err := xsdgate.Load()
	if err != nil {
		t.Fatalf("vendored sep.xsd failed to parse: %v", err)
	}
	if got, want := s.TargetNamespace, "urn:ieee:std:2030.5:ns"; got != want {
		t.Errorf("targetNamespace = %q, want %q", got, want)
	}
	if got, want := s.Version, "2.1.0"; got != want {
		t.Errorf("schema version = %q, want %q", got, want)
	}
	if n := len(s.TopLevelElements()); n != 324 {
		t.Errorf("top-level element count = %d, want 324; the vendored schema changed", n)
	}
}

// TestVendoredSchemaIntegrity pins the vendored bytes. The schema is the
// arbiter of correctness for this module, so an unnoticed edit to it would
// quietly redefine what "conformant" means.
func TestVendoredSchemaIntegrity(t *testing.T) {
	if got, want := len(schema.SEP2), 381926; got != want {
		t.Errorf("vendored sep.xsd is %d bytes, want %d; see schema/PROVENANCE.md", got, want)
	}
	if !strings.HasPrefix(string(schema.SEP2), "\xef\xbb\xbf<?xml") {
		t.Error("vendored sep.xsd lost its UTF-8 BOM; it must stay byte-faithful")
	}
	if !strings.Contains(string(schema.SEP2), "\r\n") {
		t.Error("vendored sep.xsd lost its CRLF line endings; check .gitattributes marks it -text")
	}
}

// TestEffectiveElementOrderFollowsExtension asserts the XSD extension rule
// the gate depends on: particles inherited from a base type precede the
// derived type's own particles. Get this backwards and every order check is
// wrong in the same direction.
func TestEffectiveElementOrderFollowsExtension(t *testing.T) {
	s := xsdgate.MustLoad(t)

	// Reading extends MeterReadingBase (empty) via Resource. Use a type with
	// a genuinely non-empty base: DERControl extends RandomizableEvent which
	// extends Event which extends RespondableSubscribableIdentifiedObject.
	got, err := s.EffectiveElements("DERControl")
	if err != nil {
		t.Fatalf("EffectiveElements(DERControl): %v", err)
	}
	if len(got) == 0 {
		t.Fatal("DERControl has no effective elements")
	}

	names := make([]string, len(got))
	for i, e := range got {
		names[i] = e.Name
	}
	t.Logf("DERControl effective sequence: %v", names)

	// mRID/description/version come from IdentifiedObject, far up the chain,
	// and must precede DERControl's own DERControlBase.
	idx := func(want string) int {
		for i, n := range names {
			if n == want {
				return i
			}
		}
		return -1
	}
	mrid, derBase := idx("mRID"), idx("DERControlBase")
	if mrid < 0 {
		t.Fatal("DERControl effective sequence lacks inherited mRID")
	}
	if derBase < 0 {
		t.Fatal("DERControl effective sequence lacks own DERControlBase")
	}
	if mrid >= derBase {
		t.Errorf("inherited mRID at %d must precede own DERControlBase at %d", mrid, derBase)
	}
}

// TestKnownSchemaFacts pins a handful of facts the conformance defects turned
// on, so a misparse cannot quietly make the gate agree with buggy code.
func TestKnownSchemaFacts(t *testing.T) {
	s := xsdgate.MustLoad(t)

	t.Run("ReadingType has no mRID", func(t *testing.T) {
		els, err := s.EffectiveElements("ReadingType")
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range els {
			if e.Name == "mRID" {
				t.Error("schema unexpectedly declares mRID on ReadingType")
			}
		}
	})

	t.Run("Response replyTo and responseRequired are attributes", func(t *testing.T) {
		attrs, err := s.EffectiveAttributes("RespondableResource")
		if err != nil {
			t.Fatal(err)
		}
		found := map[string]bool{}
		for _, a := range attrs {
			found[a.Name] = true
		}
		for _, want := range []string{"replyTo", "responseRequired"} {
			if !found[want] {
				t.Errorf("schema does not declare %q as an attribute on RespondableResource; attrs=%v", want, attrs)
			}
		}
	})

	t.Run("DERCapability modesSupported is required", func(t *testing.T) {
		els, err := s.EffectiveElements("DERCapability")
		if err != nil {
			t.Fatal(err)
		}
		var got *xsdgate.Element
		for i := range els {
			if els[i].Name == "modesSupported" {
				got = &els[i]
			}
		}
		if got == nil {
			t.Fatal("DERCapability has no modesSupported element")
		}
		if !got.Required {
			t.Error("modesSupported must be minOccurs=1 on DERCapability")
		}
		if got.Type != "DERControlType" {
			t.Errorf("modesSupported type = %q, want DERControlType", got.Type)
		}
	})

	t.Run("DERControlType is simpleContent over hexBinary", func(t *testing.T) {
		ct, ok := s.ComplexType("DERControlType")
		if !ok {
			t.Fatal("no DERControlType complex type")
		}
		if !ct.SimpleContent {
			t.Error("DERControlType must be a simpleContent type carrying a value, not child elements")
		}
		if ct.Base != "HexBinary32" {
			t.Errorf("DERControlType base = %q, want HexBinary32", ct.Base)
		}
		st, ok := s.SimpleType("HexBinary32")
		if !ok {
			t.Fatal("no HexBinary32 simple type")
		}
		if st.Base != "hexBinary" {
			t.Errorf("HexBinary32 base = %q, want hexBinary", st.Base)
		}
		if st.MaxLength != 4 {
			t.Errorf("HexBinary32 maxLength = %d octets, want 4", st.MaxLength)
		}
	})
}
