package sep2_test

import (
	"reflect"
	"sort"
	"strings"
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
		// DER is gated here before anything edits the struct. Two of the seven
		// link elements sep.xsd declares (AssociatedUsagePointLink and
		// CurrentDERProgramLink) have no Go field yet, and when they arrive they
		// insert at sequence positions 2 and 3 rather than appending, because
		// encoding/xml emits in struct field order. That is the defect class
		// IEEECORE-014 already fixed once for EndDevice and DERCapability, and
		// without this entry nothing would catch a wrong insertion point.
		{"DER", sep2.DER{}},
		{"Registration", sep2.Registration{}},
		{"Reading", sep2.Reading{}},
		{"ReadingType", sep2.ReadingType{}},
		{"MirrorMeterReading", sep2.MirrorMeterReading{}},
		// LogEvent and its list are gated by IEEECORE-084, which put the
		// function set on a client-reachable address for the first time. A
		// resource nothing could reach was a resource nothing could be wrong
		// about; now that an EndDevice advertises the list and a device POSTs
		// alarms into it, the wire form is load-bearing and belongs here. The
		// list is gated as well as the member because a LogEventList carries
		// its own paging attributes, and an all or results emitted as an
		// element rather than an attribute is invisible to a round trip.
		{"LogEvent", sep2.LogEvent{}},
		{"LogEventList", sep2.LogEventList{}},
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
			// A POPULATED LogEvent, because the zero value leaves details and
			// extendedData absent (both are omitempty) and the marshalled
			// check then has nothing to inspect for either. extendedData is
			// the one that matters: sep.xsd types it UInt32 while the Go field
			// is a *int64, so a negative value marshals to a document the
			// schema rejects. The gate catches that lexically, which is why a
			// populated fixture is the entry and not the zero value.
			typeName: "LogEvent",
			v:        populatedLogEvent(),
		},
		{
			// The list carries its own paging attributes, and pollRate is
			// stamped by the server rather than by any client, so a fixture
			// that leaves them at zero asserts nothing about how they reach
			// the wire.
			typeName: "LogEventList",
			v: sep2.LogEventList{
				ListResource: sep2.ListResource{
					SubscribableResource: sep2.SubscribableResource{
						Resource: sep2.Resource{Href: "/edev/1/lel"},
					},
					All:      1,
					Results:  1,
					PollRate: 900,
				},
				LogEvent: []sep2.LogEvent{populatedLogEvent()},
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

// populatedLogEvent is the fixture shared by the LogEvent and LogEventList
// entries above, so the member a client reads inside the list and the member it
// reads at its own href are gated as the same document.
//
// details is 18 characters. sep.xsd types it String32, and this gate does not
// enforce maxLength, so a longer fixture would pass here while being invalid on
// the standard's terms; keeping it short means the fixture is not itself the
// thing that is wrong.
func populatedLogEvent() sep2.LogEvent {
	extendedData := int64(9007)
	return sep2.LogEvent{
		Resource:        sep2.Resource{Href: "/edev/1/lel/00000000001604963587"},
		CreatedDateTime: 1604963587,
		Details:         "gen software alarm",
		ExtendedData:    &extendedData,
		FunctionSet:     sep2.FunctionSetLogEvent,
		LogEventCode:    27,
		LogEventID:      7,
		LogEventPEN:     54465,
		ProfileID:       2,
	}
}

// ---------------------------------------------------------------------------
// RespondableResource: the IEEECORE-103 class
// ---------------------------------------------------------------------------

// respondableTypes are the concrete resources that inherit replyTo and
// responseRequired from the schema's RespondableResource, via Go's Event
// embed. Every one of them is affected by any defect in how those two fields
// are modelled, because all four share the single declaration on sep2.Event.
//
// DERControl is the reason this list exists. Until IEEECORE-103 it appeared
// NOWHERE in this file: not in the clean table, not in the known-failure
// table, and not in TestSchemaGateCoversKnownResources' required list. The
// resource whose wire format broke the EPRI reference client was the one
// resource in the DER function set that no schema-gated test ever looked at.
func respondableFixtures() map[string]any {
	replyTo := "/rsps/1/rsp"
	rr := sep2.HexBinary8(0x07)
	category := sep2.DeviceCategoryType(0x0080)

	base := func() sep2.RandomizableEvent {
		var e sep2.RandomizableEvent
		e.ReplyTo = replyTo
		e.ResponseRequired = &rr
		e.MRID = "0102030405060708090A0B0C0D0E0F10"
		e.Description = "populated fixture"
		e.CreationTime = 1500000000
		e.EventStatus = &sep2.EventStatus{CurrentStatus: 1, DateTime: 1500000000}
		e.Interval = &sep2.DateTimeInterval{Duration: 3600, Start: 1500000000}
		return e
	}

	derc := sep2.DERControl{RandomizableEvent: base()}
	derc.Href = "/derp/0/derc/1"
	derc.DERControlBase = &sep2.DERControlBase{RampTms: func() *uint16 { v := uint16(30); return &v }()}

	edc := sep2.EndDeviceControl{RandomizableEvent: base()}
	edc.Href = "/drp/0/edc/1"
	edc.DeviceCategory = &category

	frr := sep2.FlowReservationResponse{RandomizableEvent: base()}
	frr.Href = "/edev/1/frp/1"

	tm := sep2.TextMessage{RandomizableEvent: base()}
	tm.Href = "/msg/0/txt/1"

	return map[string]any{
		"DERControl":              derc,
		"EndDeviceControl":        edc,
		"FlowReservationResponse": frr,
		"TextMessage":             tm,
	}
}

// TestSchemaGatePopulatedRespondableResources is the check that would have
// caught IEEECORE-103 before it shipped, and it is deliberately built on
// POPULATED fixtures.
//
// The zero-value lane cannot see this defect class at all. Both fields are
// omitempty, so at the zero value ReplyTo is "" and ResponseRequired is nil,
// neither reaches the wire, and the marshalled document that the gate
// validates is one in which the defect is structurally invisible. Every
// existing marshal-lane pin for a respondable resource was therefore
// validating a document that could not disagree with the schema about these
// two fields no matter how they were tagged. That is the same shape as the
// IEEECORE-055 blind spot recorded on the DERStatus fixture above: a gate
// reading an empty document and the emptiness being mistaken for coverage.
//
// The assertion is over the SERIALIZED BYTES rather than a round trip. A
// marshal-then-unmarshal test through this package's own encoder and decoder
// passes identically whether these fields are attributes or child elements,
// because encoding/xml decodes whatever encoding/xml produced. Such a test is
// green in BOTH the correct and the broken world and can never gate this
// class. The EPRI reference client is a foreign parser and caught the defect
// on its first run; short of running one in CI, asserting bytes is the
// substitute.
func TestSchemaGatePopulatedRespondableResources(t *testing.T) {
	for typeName, v := range respondableFixtures() {
		t.Run(typeName, func(t *testing.T) {
			// Lane 1: the struct definition must not place either field
			// as a child element.
			for _, p := range xsdgate.CollectStructProblems(t, typeName, v).Summary() {
				if strings.HasPrefix(p, "placement ") &&
					(strings.HasSuffix(p, ".ReplyTo") || strings.HasSuffix(p, ".ResponseRequired")) {
					t.Errorf("IEEECORE-103 regression: %s", p)
				}
			}

			// Lane 2: the bytes. The schema must not report either name as
			// an element it does not declare, which is what it reports when
			// a document carries <replyTo> or <responseRequired> children.
			problems, data := xsdgate.CollectProblems(t, typeName, v)
			for _, p := range problems.Summary() {
				if strings.Contains(p, "replyTo") || strings.Contains(p, "responseRequired") {
					t.Errorf("IEEECORE-103 regression in marshalled %s: %s\nXML:\n%s", typeName, p, data)
				}
			}

			// Lane 3: the literal wire text, which is the only lane that
			// pins the lexical form. responseRequired is a HexBinary8 and
			// must reach the wire as hex "07"; in attribute position that
			// depends on MarshalXMLAttr, and without it encoding/xml would
			// render decimal "7" that a hexBinary parser silently reads as
			// 0x07's neighbour rather than erroring.
			for _, want := range []string{` replyTo="/rsps/1/rsp"`, ` responseRequired="07"`} {
				if !strings.Contains(string(data), want) {
					t.Errorf("%s must carry %s on its start tag; XML:\n%s", typeName, want, data)
				}
			}
			for _, forbidden := range []string{"<replyTo>", "<responseRequired>"} {
				if strings.Contains(string(data), forbidden) {
					t.Errorf("%s emitted %s as a child element (IEEECORE-103); XML:\n%s", typeName, forbidden, data)
				}
			}
		})
	}
}

// TestSchemaGateRejectsRespondableFieldsAsElements pins the gate's ability to
// catch IEEECORE-103, permanently and independently of the struct.
//
// The bytes below are the exact defective wire form this server emitted
// before the fix, transcribed from the DERControlList that made the EPRI
// reference client abort with "parse error in message body / parse_stack: 0
// DERControlList 1 DERControl". Validating a literal fixture rather than a
// marshalled struct is deliberate and is the same technique
// TestSchemaGateDetectsScalarForComplexType uses: a test built on the struct
// silently stops testing the moment the struct is correct, whereas these
// bytes keep their meaning forever.
//
// This is the RED proof asked for on IEEECORE-103. Reverting the two ,attr
// tags on sep2.Event makes TestSchemaGatePopulatedRespondableResources fail;
// this test fails only if the gate ITSELF loses the ability to tell the two
// encodings apart, which is the deeper regression.
func TestSchemaGateRejectsRespondableFieldsAsElements(t *testing.T) {
	const defective = `<DERControl xmlns="urn:ieee:std:2030.5:ns" href="/derp/0/derc/1">
  <replyTo>/rsps/1/rsp</replyTo>
  <responseRequired>07</responseRequired>
  <mRID>0102030405060708090A0B0C0D0E0F10</mRID>
  <creationTime>1500000000</creationTime>
</DERControl>`

	s := xsdgate.MustLoad(t)
	problems, err := s.Validate("DERControl", []byte(defective))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	got := problems.Summary()
	want := []string{
		"placement DERControl/replyTo",
		"placement DERControl/responseRequired",
	}
	for _, w := range want {
		found := false
		for _, g := range got {
			if g == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("the schema gate no longer reports %q for the pre-IEEECORE-103 wire form.\n"+
				"That form is what broke the EPRI reference client, so a gate that accepts it "+
				"cannot protect this class.\nobserved: %v\ndocument:\n%s", w, got, defective)
		}
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
		// The three resources IEEECORE-081 put on the wire as addressable
		// instances. Their lists were already served, so the wire format is not
		// new, but the instance routes make each one fetchable on its own and
		// that is the point at which an unmodelled element becomes a client's
		// problem rather than a list-shaped curiosity. Pinned rather than fixed:
		// dropping omitempty changes the wire format of shipped resources and
		// belongs in its own reviewed change, one resource at a time.
		{
			typeName: "FlowReservationRequest",
			zero:     sep2.FlowReservationRequest{},
			wantStruct: []string{
				"omitempty-required FlowReservationRequest.EnergyRequested",
				"omitempty-required FlowReservationRequest.IntervalRequested",
				"omitempty-required FlowReservationRequest.MRID",
				"omitempty-required FlowReservationRequest.PowerRequested",
				"omitempty-required FlowReservationRequest.RequestStatus",
			},
			wantMarshal: []string{
				"missing-element FlowReservationRequest/RequestStatus",
				"missing-element FlowReservationRequest/energyRequested",
				"missing-element FlowReservationRequest/intervalRequested",
				"missing-element FlowReservationRequest/mRID",
				"missing-element FlowReservationRequest/powerRequested",
			},
			reason: "every element the schema requires beyond creationTime is tagged omitempty, " +
				"so a request a client POSTs without them round-trips as a document carrying " +
				"only creationTime. The handler fills none of them either: POST /edev/{id}/frq " +
				"stamps href and creationTime and stores whatever else the client sent.",
		},
		{
			typeName: "FlowReservationResponse",
			zero:     sep2.FlowReservationResponse{},
			wantStruct: []string{
				"omitempty-required FlowReservationResponse.EnergyAvailable",
				"omitempty-required FlowReservationResponse.EventStatus",
				"omitempty-required FlowReservationResponse.Interval",
				"omitempty-required FlowReservationResponse.MRID",
				"omitempty-required FlowReservationResponse.PowerAvailable",
				"omitempty-required FlowReservationResponse.Subject",
				"unknown-element FlowReservationResponse.RandomizeDuration",
				"unknown-element FlowReservationResponse.RandomizeStart",
			},
			wantMarshal: []string{
				"missing-element FlowReservationResponse/EventStatus",
				"missing-element FlowReservationResponse/energyAvailable",
				"missing-element FlowReservationResponse/interval",
				"missing-element FlowReservationResponse/mRID",
				"missing-element FlowReservationResponse/powerAvailable",
				"missing-element FlowReservationResponse/subject",
			},
			reason: "one defect beyond the usual omitempty set, from the embedded " +
				"RandomizableEvent: the schema derives FlowReservationResponse from Event, not " +
				"RandomizableEvent, so randomizeStart and randomizeDuration are elements the " +
				"schema does not declare here. That is IEEECORE-098 and is NOT fixed here. " +
				"This entry previously also pinned 'placement ReplyTo' and 'placement " +
				"ResponseRequired', which was this gate correctly reporting IEEECORE-103 and " +
				"the pin turning that report into an expected value. IEEECORE-103 fixed it, so " +
				"those two are gone. See TestSchemaGateRejectsRespondableFieldsAsElements for " +
				"the guard that now keeps them gone.",
		},
		{
			typeName: "TextMessage",
			zero:     sep2.TextMessage{},
			wantStruct: []string{
				"omitempty-required TextMessage.EventStatus",
				"omitempty-required TextMessage.Interval",
				"omitempty-required TextMessage.MRID",
				"unknown-element TextMessage.RandomizeDuration",
				"unknown-element TextMessage.RandomizeStart",
			},
			wantMarshal: []string{
				"missing-element TextMessage/EventStatus",
				"missing-element TextMessage/interval",
				"missing-element TextMessage/mRID",
			},
			reason: "the same RandomizableEvent-versus-Event divergence as " +
				"FlowReservationResponse (IEEECORE-098, not fixed here): the schema derives " +
				"TextMessage from Event. The missing EventStatus is the absent-element half of " +
				"IEEECORE-044, which already records that nothing on the TextMessage path ever " +
				"constructs one. The two 'placement' entries this list used to carry were " +
				"IEEECORE-103 and are fixed.",
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
	//
	// The four RespondableResource types were added by IEEECORE-103. DERControl
	// in particular was absent from every list in this file while being the
	// single most control-critical resource the server serves, which is how a
	// wire-format defect on it reached a released tag with this gate green in
	// CI. A gate's coverage list is only as good as the argument for what is
	// on it, so the rule now is explicit: every resource this server SERVES is
	// in scope, not only the ones an old card happened to enumerate.
	//
	// LogEvent and LogEventList were added by IEEECORE-084. They were out of
	// scope for the old enumeration because the function set was unreachable:
	// it was served at /edev/{id}/log while the WADL declares /edev/{id}/lel,
	// and no EndDevice advertised a LogEventListLink at either address. That is
	// no longer true, so the rule stated above applies to them: this server
	// serves them, therefore they are in scope.
	required := []string{
		"DERCapability", "DERSettings", "DERStatus",
		"MirrorUsagePoint", "MirrorMeterReading", "Registration",
		"EndDevice", "Reading", "ReadingType",
		"DERControl", "EndDeviceControl", "FlowReservationResponse", "TextMessage",
		"LogEvent", "LogEventList",
	}

	covered := map[string]bool{
		"Registration": true, "Reading": true, "ReadingType": true,
		"MirrorMeterReading": true, "DERCapability": true, "DERSettings": true,
		"DERStatus": true, "MirrorUsagePoint": true, "UsagePoint": true,
		"EndDevice": true, "DERControl": true, "EndDeviceControl": true,
		"FlowReservationResponse": true, "TextMessage": true,
		"LogEvent": true, "LogEventList": true,
	}

	// Appearing in the zero-value tables is NOT full coverage. A resource
	// whose optional fields are all nil pointers marshals to almost nothing,
	// so the marshalled check has no values to inspect and a wrong Go type
	// behind an optional element stays invisible. That is exactly how
	// IEEECORE-055 reached an interop run. Resources listed here must have a
	// populated fixture in TestSchemaGatePopulatedResources.
	populated := []string{
		"Registration", "Reading", "ReadingType", "MirrorMeterReading", "DERStatus",
		"DERControl", "EndDeviceControl", "FlowReservationResponse", "TextMessage",
		"LogEvent", "LogEventList",
	}

	// The respondable types must be populated, not merely present. Their two
	// respondable fields are omitempty, so a zero-value entry marshals a
	// document with neither field in it and asserts nothing about how they are
	// encoded. Requiring the fixture here is what stops a future edit from
	// "covering" them with a zero value and reintroducing the blind spot.
	respondable := respondableFixtures()
	for _, name := range []string{"DERControl", "EndDeviceControl", "FlowReservationResponse", "TextMessage"} {
		if _, ok := respondable[name]; !ok {
			t.Errorf("respondable resource %q has no populated fixture; the zero value cannot "+
				"exercise replyTo or responseRequired because both are omitempty", name)
		}
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
