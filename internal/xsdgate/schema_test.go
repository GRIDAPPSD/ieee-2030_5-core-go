package xsdgate_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/internal/xsdgate"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/schema"
)

// TestSchemaGateArmed is the canary that tells "the gate ran" apart from
// "the gate was skipped", without parsing prose out of go test output. It is
// the single test name to look for: PASS means a schema was located,
// verified against its digest, and parsed, so the rest of the gate really
// validated something; SKIP means no schema was available and NOTHING in
// this module checked any resource against the standard.
//
// Under schema.EnvRequired an absent schema fails here instead of skipping,
// so a run that is supposed to supply a schema cannot report green without
// one.
func TestSchemaGateArmed(t *testing.T) {
	s := xsdgate.MustLoad(t)

	path, err := schema.Resolve()
	if err != nil {
		t.Fatalf("resolve schema path after a successful load: %v", err)
	}
	t.Logf("schema gate armed: %s, %d top-level elements", path, len(s.TopLevelElements()))
}

// TestSchemaParses is the load-bearing guard on the whole gate. If the
// schema ever gains a construct outside the supported subset, ParseSchema
// errors here rather than letting the gate silently check less than its
// documentation claims.
func TestSchemaParses(t *testing.T) {
	s := xsdgate.MustLoad(t)

	if got, want := s.TargetNamespace, schema.Namespace; got != want {
		t.Errorf("targetNamespace = %q, want %q", got, want)
	}
	if got, want := s.Version, "2.1.0"; got != want {
		t.Errorf("schema version = %q, want %q", got, want)
	}
	if n := len(s.TopLevelElements()); n != 324 {
		t.Errorf("top-level element count = %d, want 324; this is not the expected schema", n)
	}
}

// TestSchemaIntegrity pins the bytes the gate reads. The schema is the
// arbiter of correctness for this module, so a different edition or an
// edited copy would quietly redefine what "conformant" means. Pinning it
// turns that into one clear failure instead of a spray of confusing
// validation errors.
//
// The digest is recomputed here rather than trusting schema.Load's own
// check, so weakening or disabling the loader's verification cannot leave
// the identity of the document unasserted.
//
// The predecessor of this test asserted a byte length, a UTF-8 BOM, and CRLF
// line endings on a vendored file. A digest over the normalized form is a
// strictly stronger identity check, and it does not fail an operator whose
// copy was extracted with converted line endings.
func TestSchemaIntegrity(t *testing.T) {
	xsdgate.MustLoad(t) // applies the skip-or-fail policy when no schema exists

	data, path, err := schema.Load()
	if err != nil {
		t.Fatalf("load IEEE 2030.5 schema: %v", err)
	}
	norm := schema.Normalize(data)
	sum := sha256.Sum256(norm)
	if got, want := hex.EncodeToString(sum[:]), schema.NormalizedSHA256; got != want {
		t.Errorf("%s: normalized sha256 = %s, want %s; see schema/PROVENANCE.md", path, got, want)
	}
	if got, want := len(norm), schema.NormalizedSize; got != want {
		t.Errorf("%s: normalized size = %d bytes, want %d", path, got, want)
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

	t.Run("DERCurve required elements in sequence", func(t *testing.T) {
		els, err := s.EffectiveElements("DERCurve")
		if err != nil {
			t.Fatal(err)
		}
		assertSequence(t, "DERCurve", els, []seqWant{
			{name: "mRID", required: true},
			{name: "creationTime", typ: "TimeType", required: true},
			{name: "CurveData", required: true},
			{name: "curveType", required: true},
			{name: "xMultiplier", typ: "PowerOfTenMultiplierType", required: true},
			{name: "yMultiplier", typ: "PowerOfTenMultiplierType", required: true},
			{name: "yRefType", typ: "DERUnitRefType", required: true},
		})
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

// seqWant is one element assertSequence expects; typ "" leaves the declared
// type unchecked.
type seqWant struct {
	name, typ string
	required  bool
}

// assertSequence fails for each wanted element that is undeclared, has the
// wrong minOccurs or type, or sits out of the listed order in els.
func assertSequence(t *testing.T, typeName string, els []xsdgate.Element, want []seqWant) {
	t.Helper()
	pos := make(map[string]int, len(els))
	for i, e := range els {
		pos[e.Name] = i
	}
	last, lastName := -1, ""
	for _, w := range want {
		i, ok := pos[w.name]
		if !ok {
			t.Errorf("%s declares no %s element", typeName, w.name)
			continue
		}
		e := els[i]
		if e.Required != w.required {
			t.Errorf("%s/%s required = %v, want %v", typeName, w.name, e.Required, w.required)
		}
		if w.typ != "" && e.Type != w.typ {
			t.Errorf("%s/%s type = %q, want %q", typeName, w.name, e.Type, w.typ)
		}
		if i < last {
			t.Errorf("%s/%s is at sequence position %d, before %s at %d", typeName, w.name, i, lastName, last)
			continue
		}
		last, lastName = i, w.name
	}
}
