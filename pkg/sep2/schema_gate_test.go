package sep2_test

import (
	"reflect"
	"sort"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/internal/xsdgate"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// This file gates the wire format of sep2 resources against the normative
// IEEE 2030.5 schema (sep.xsd), which is not distributed with this project
// and is read at test time from an operator-supplied copy: see the NOTICE
// file at the repository root. Every test here SKIPS when no copy is
// available. Before this file existed, every
// conformance defect in this package was found by a human reading the XSD
// against struct tags. None was caught by a test, because round-tripping
// through encoding/xml cannot detect wrong element order, dropped required
// elements, undeclared elements, or a value in the wrong lexical form.
//
// Two complementary checks run against each resource:
//
//   - CheckStruct inspects the struct DEFINITION. It sees every field
//     whether or not a test populates it, so it catches ordering defects
//     between optional fields and omitempty on required elements.
//   - Validate inspects MARSHALLED OUTPUT. It sees what actually goes on the
//     wire, so it catches required elements that have no Go field at all,
//     plus lexical defects in real values.
//
// Neither subsumes the other. DERSettings demonstrates why: the struct check
// finds two omitempty-on-required fields, while the marshalled check finds a
// third required element (setGradW) that the struct simply does not model.

// ---------------------------------------------------------------------------
// Conformant resources
// ---------------------------------------------------------------------------

// TestSchemaGateCleanResources asserts that resources currently conformant
// STAY conformant. This is the half of the gate that does forward work: a
// future edit that reorders a field or adds an omitempty to a required
// element fails here.
func TestSchemaGateCleanResources(t *testing.T) {
	tests := []struct {
		typeName string
		zero     any
	}{
		{"Registration", sep2.Registration{}},
		{"Reading", sep2.Reading{}},
		{"ReadingType", sep2.ReadingType{}},
		{"MirrorMeterReading", sep2.MirrorMeterReading{}},
	}

	for _, tc := range tests {
		t.Run(tc.typeName+"/struct", func(t *testing.T) {
			xsdgate.AssertStructValid(t, tc.typeName, tc.zero)
		})
		// The zero value is the case that exposes omitempty dropping a
		// required element, so it is checked explicitly rather than only
		// checking a conveniently fully-populated fixture.
		t.Run(tc.typeName+"/marshal-zero", func(t *testing.T) {
			xsdgate.AssertValid(t, tc.typeName, tc.zero)
		})
	}
}

// TestSchemaGatePopulatedResources runs the gate over realistic values. The
// zero-value cases above cannot exercise lexical checks, since a dropped
// element has no value to inspect. These fixtures deliberately populate
// hexBinary and bounded-integer fields, which is where silent wire defects
// have actually occurred.
func TestSchemaGatePopulatedResources(t *testing.T) {
	qualityFlags := sep2.HexBinary16(0x0A0B)
	value := int64(4242)
	accum := uint8(9)
	uom := uint8(38)
	powerOfTen := int8(-3)
	alarm := sep2.HexBinary32(0x01)

	tests := []struct {
		typeName string
		v        any
	}{
		{
			// DERStatus is pinned as a known failure below, but only at its
			// ZERO value, where every optional field is a nil pointer and so
			// never reaches the wire. That blind spot is how IEEECORE-055 got
			// in: stateOfChargeStatus was modelled as a bare *uint16 against a
			// schema complexType, and no fixture ever populated it, so the
			// marshalled check had nothing to inspect. This entry populates
			// every field, including readingTime, which is the resource's one
			// pinned defect and is therefore clean here.
			typeName: "DERStatus",
			v: sep2.DERStatus{
				AlarmStatus:           &alarm,
				GenConnectStatus:      &sep2.ConnectStatusType{DateTime: 1604963587, Value: 1},
				InverterStatus:        &sep2.InverterStatusType{DateTime: 1604963587, Value: 2},
				OperationalModeStatus: &sep2.OperationalModeStatusType{DateTime: 1604963587, Value: 2},
				ReadingTime:           1604963587,
				StateOfChargeStatus:   &sep2.StateOfChargeStatusType{DateTime: 1604963587, Value: 7500},
				StorageModeStatus:     &sep2.StorageModeStatusType{DateTime: 1604963587, Value: 1},
			},
		},
		{
			typeName: "Registration",
			v: sep2.Registration{
				Resource:           sep2.Resource{Href: "/edev/1/rg"},
				DateTimeRegistered: 1500000000,
				PIN:                123456,
				PollRate:           900,
			},
		},
		{
			typeName: "Reading",
			v: sep2.Reading{
				Resource:     sep2.Resource{Href: "/upt/0/mr/0/r/1"},
				QualityFlags: &qualityFlags,
				Value:        &value,
			},
		},
		{
			typeName: "ReadingType",
			v: sep2.ReadingType{
				AccumulationBehaviour: &accum,
				PowerOfTenMultiplier:  &powerOfTen,
				Uom:                   &uom,
			},
		},
		{
			typeName: "MirrorMeterReading",
			v: sep2.MirrorMeterReading{
				MRID:           "0102030405060708090A0B0C0D0E0F10",
				Description:    "site meter",
				LastUpdateTime: 1500000000,
				Reading: &sep2.Reading{
					QualityFlags: &qualityFlags,
					Value:        &value,
				},
				ReadingType: &sep2.ReadingType{Uom: &uom},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.typeName, func(t *testing.T) {
			xsdgate.AssertValid(t, tc.typeName, tc.v)
		})
	}
}

// ---------------------------------------------------------------------------
// Known-failing resources
// ---------------------------------------------------------------------------

// knownFailure pins the exact set of schema violations a resource currently
// has.
//
// Pinning rather than skipping is deliberate. A skipped resource is invisible
// and drifts; a pinned one is enumerated in code, is asserted on every run,
// and fails loudly in BOTH directions: if a new defect appears, and if
// someone fixes a defect without unpinning it. The gate lands green while
// every open defect stays in the build's field of view.
//
// These defects are NOT fixed here. Fixing them changes the wire format of
// shipped resources and belongs in its own reviewed change, one resource at
// a time. See the per-entry reason for what each one needs.
type knownFailure struct {
	typeName string
	zero     any
	// wantStruct and wantMarshal are "kind path" summaries, sorted.
	wantStruct  []string
	wantMarshal []string
	reason      string
}

func TestSchemaGateKnownFailures(t *testing.T) {
	cases := []knownFailure{
		{
			typeName: "DERCapability",
			zero:     sep2.DERCapability{},
			wantStruct: []string{
				"omitempty-required DERCapability.ModesSupported",
				"omitempty-required DERCapability.RTGMaxW",
				"omitempty-required DERCapability.Type",
			},
			wantMarshal: []string{
				"missing-element DERCapability/modesSupported",
				"missing-element DERCapability/rtgMaxW",
				"missing-element DERCapability/type",
			},
			reason: "modesSupported, rtgMaxW and type are minOccurs=1 but tagged omitempty, " +
				"so a zero-value DERCapability serializes with none of them. Fixing this means " +
				"dropping omitempty and deciding each field's zero-value semantics.",
		},
		{
			typeName: "DERSettings",
			zero:     sep2.DERSettings{},
			wantStruct: []string{
				"omitempty-required DERSettings.SetMaxW",
				"omitempty-required DERSettings.UpdatedTime",
			},
			wantMarshal: []string{
				"missing-element DERSettings/setGradW",
				"missing-element DERSettings/setMaxW",
				"missing-element DERSettings/updatedTime",
			},
			reason: "setMaxW and updatedTime are omitempty on required elements. setGradW is " +
				"required by the schema and has NO Go field at all, which is why the marshalled " +
				"check reports three problems where the struct check reports two.",
		},
		{
			typeName: "DERStatus",
			zero:     sep2.DERStatus{},
			wantStruct: []string{
				"omitempty-required DERStatus.ReadingTime",
			},
			wantMarshal: []string{
				"missing-element DERStatus/readingTime",
			},
			reason: "readingTime is minOccurs=1 but omitempty, so a DERStatus reporting at " +
				"epoch zero, or one never explicitly stamped, omits the timestamp entirely.",
		},
		{
			typeName: "MirrorUsagePoint",
			zero:     sep2.MirrorUsagePoint{},
			wantStruct: []string{
				"omitempty-required MirrorUsagePoint.DeviceLFDI",
				"omitempty-required MirrorUsagePoint.MRID",
			},
			wantMarshal: []string{
				"missing-element MirrorUsagePoint/deviceLFDI",
				"missing-element MirrorUsagePoint/mRID",
			},
			reason: "mRID (from IdentifiedObject) and deviceLFDI are both minOccurs=1 but " +
				"omitempty. Note the neighbouring roleFlags field already carries a comment " +
				"explaining precisely this hazard, and was fixed, while these two were missed: " +
				"exactly the gap a mechanical gate closes.",
		},
		{
			typeName: "UsagePoint",
			zero:     sep2.UsagePoint{},
			wantStruct: []string{
				"omitempty-required UsagePoint.MRID",
				"omitempty-required UsagePoint.RoleFlags",
				"unknown-attribute UsagePoint.Subscribable",
			},
			wantMarshal: []string{
				"missing-element UsagePoint/mRID",
				"missing-element UsagePoint/roleFlags",
			},
			reason: "mRID and roleFlags are omitempty on required elements. Separately, the Go " +
				"type embeds SubscribableResource, but the schema derives UsagePoint through " +
				"UsagePointBase -> IdentifiedObject -> Resource, none of which declare the " +
				"subscribable attribute. The schema has a distinct SubscribableIdentifiedObject " +
				"for that, and UsagePoint does not use it, so emitting subscribable puts an " +
				"undeclared attribute on the wire.",
		},
		{
			typeName: "EndDevice",
			zero:     sep2.EndDevice{},
			// The struct definition itself is well-formed: field order and
			// placement are correct, hence no struct problems.
			wantStruct: nil,
			wantMarshal: []string{
				"lexical EndDevice/sFDI",
			},
			reason: "sFDI is schema type UInt40 (xs:unsignedLong) but is modelled as a Go " +
				"string with no omitempty, so an unset SFDI serializes as an empty element, " +
				"which is not a valid unsignedLong. Any non-numeric string would serialize " +
				"just as invalidly; the Go type does not constrain it.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.typeName, func(t *testing.T) {
			t.Logf("KNOWN-FAILING, not fixed in this change: %s", tc.reason)

			gotStruct := xsdgate.CollectStructProblems(t, tc.typeName, tc.zero).Summary()
			assertPinned(t, "struct-definition", tc.typeName, gotStruct, tc.wantStruct)

			gotMarshal, xmlBytes := xsdgate.CollectProblems(t, tc.typeName, tc.zero)
			assertPinned(t, "marshalled-output", tc.typeName, gotMarshal.Summary(), tc.wantMarshal)
			t.Logf("zero-value %s serializes as:\n%s", tc.typeName, xmlBytes)
		})
	}
}

// assertPinned compares an observed problem set against the pinned one and
// explains, in the failure message, which direction the drift went and what
// to do about it. A pinned expectation that fails without telling the reader
// whether things got better or worse is a trap for whoever hits it next.
func assertPinned(t *testing.T, lane, typeName string, got, want []string) {
	t.Helper()

	gotSorted := append([]string(nil), got...)
	wantSorted := append([]string(nil), want...)
	sort.Strings(gotSorted)
	sort.Strings(wantSorted)

	if reflect.DeepEqual(gotSorted, wantSorted) {
		return
	}

	inWant := map[string]bool{}
	for _, w := range wantSorted {
		inWant[w] = true
	}
	inGot := map[string]bool{}
	for _, g := range gotSorted {
		inGot[g] = true
	}

	var fixed, added []string
	for _, w := range wantSorted {
		if !inGot[w] {
			fixed = append(fixed, w)
		}
	}
	for _, g := range gotSorted {
		if !inWant[g] {
			added = append(added, g)
		}
	}

	if len(added) > 0 {
		t.Errorf("%s %s: NEW schema violation(s) appeared: %v\n"+
			"This is a regression. Fix the resource rather than widening the pin.\n"+
			"pinned: %v\nobserved: %v", typeName, lane, added, wantSorted, gotSorted)
	}
	if len(fixed) > 0 {
		t.Errorf("%s %s: pinned violation(s) no longer occur: %v\n"+
			"If you fixed the resource, remove them from the pinned list in this test "+
			"(and drop the whole entry once its lists are empty, moving the type into "+
			"TestSchemaGateCleanResources).\npinned: %v\nobserved: %v",
			typeName, lane, fixed, wantSorted, gotSorted)
	}
}

// TestSchemaGateCoversKnownResources guards the gate's own coverage. Adding a
// resource to the package without adding it here would leave it ungated, and
// an ungated resource is exactly how the current defects got in.
func TestSchemaGateCoversKnownResources(t *testing.T) {
	// Every resource named in the IEEECORE-048 scope must appear in one of
	// the two tables above.
	required := []string{
		"DERCapability", "DERSettings", "DERStatus",
		"MirrorUsagePoint", "MirrorMeterReading", "Registration",
		"EndDevice", "Reading", "ReadingType",
	}

	covered := map[string]bool{
		"Registration": true, "Reading": true, "ReadingType": true,
		"MirrorMeterReading": true, "DERCapability": true, "DERSettings": true,
		"DERStatus": true, "MirrorUsagePoint": true, "UsagePoint": true,
		"EndDevice": true,
	}

	// Appearing in the zero-value tables is NOT full coverage. A resource
	// whose optional fields are all nil pointers marshals to almost nothing,
	// so the marshalled check has no values to inspect and a wrong Go type
	// behind an optional element stays invisible. That is exactly how
	// IEEECORE-055 reached an interop run. Resources listed here must have a
	// populated fixture in TestSchemaGatePopulatedResources.
	populated := []string{
		"Registration", "Reading", "ReadingType", "MirrorMeterReading", "DERStatus",
	}

	s := xsdgate.MustLoad(t)
	for _, name := range required {
		if !covered[name] {
			t.Errorf("resource %q is in scope but not gated", name)
		}
		if _, ok := s.ComplexType(name); !ok {
			t.Errorf("resource %q is not a complexType in the IEEE 2030.5 schema", name)
		}
	}
	for _, name := range populated {
		if _, ok := s.ComplexType(name); !ok {
			t.Errorf("resource %q needs a populated fixture but is not a complexType in the vendored schema", name)
		}
	}
}

// TestSchemaGateDetectsScalarForComplexType pins the gate's ability to catch
// the IEEECORE-055 defect class: a schema complexType modelled in Go as a bare
// scalar, so the element carries chardata instead of the required child
// elements.
//
// It validates raw bytes rather than a marshalled struct on purpose. A test
// built on the struct would silently stop testing this the moment the struct
// is correct; these bytes are the exact defective wire form the server emitted
// before the fix, so the assertion keeps its meaning permanently. The mirror
// image of this defect is what broke the PUT path: encoding/xml could not
// parse a spec-correct client's nested stateOfChargeStatus into a *uint16 and
// the server answered 400.
func TestSchemaGateDetectsScalarForComplexType(t *testing.T) {
	const defective = `<DERStatus xmlns="urn:ieee:std:2030.5:ns">
  <readingTime>1604963587</readingTime>
  <stateOfChargeStatus>7500</stateOfChargeStatus>
</DERStatus>`

	s := xsdgate.MustLoad(t)
	problems, err := s.Validate("DERStatus", []byte(defective))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	want := []string{
		"missing-element DERStatus/stateOfChargeStatus/dateTime",
		"missing-element DERStatus/stateOfChargeStatus/value",
	}
	assertPinned(t, "marshalled-output", "DERStatus(defective-fixture)", problems.Summary(), want)
}
