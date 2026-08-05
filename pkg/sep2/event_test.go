// Package sep2_test covers the Event-base wire format for `replyTo` and
// `responseRequired`, which IEEE 2030.5 section 10.1.3 (Event rules) and the
// XSD `RespondableResource` declare as ATTRIBUTES (sep.xsd:5435, sep.xsd:5440).
//
// These tests gate the OnTransition hook-wiring work
// (GRIDAPPSD/ieee-2030_5-server-go#109): the hook reads
// `ReplyTo` off a decoded DERControl to drive `(*SEP2Client).
// PostResponse`, and reads the `ResponseRequired` bitmap to decide
// which transition statuses warrant a Response POST.
//
// A ROUND-TRIP TEST CANNOT GATE THIS FILE'S SUBJECT MATTER. Marshalling a
// DERControl and unmarshalling it back with our own encoder and decoder
// passes identically whether the two fields are encoded as attributes or as
// child elements, because our decoder accepts whatever our encoder produced.
// That symmetry is exactly how a real defect shipped: the round-trip tests
// below were green against the element encoding that made the EPRI reference
// client abort its parse. Only an assertion over the SERIALIZED BYTES, or a
// foreign parser, distinguishes the two. Every test here that exists to gate
// the encoding therefore asserts on the marshalled bytes; the round-trip
// assertions are kept only to cover value fidelity, which is a different
// property.
package sep2_test

import (
	"encoding/xml"
	"fmt"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// TestEventReplyToRoundTrip asserts that a DERControl marshalled with
// ReplyTo set survives an unmarshal-marshal-unmarshal cycle without
// losing or mutating the URI. Both relative ("/edev/1/rsps/1/rsp")
// and absolute ("https://example/rsp") forms are exercised: the
// OnTransition hook resolves both via SEP2Client.resolveServerURL.
func TestEventReplyToRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		replyTo string
	}{
		{name: "relative_uri", replyTo: "/edev/1/rsps/1/rsp"},
		{name: "absolute_uri", replyTo: "https://example.invalid/rsps/1/rsp"},
		{name: "empty_omits_element", replyTo: ""},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := sep2.DERControl{}
			ctrl.MRID = "DERC-001"
			ctrl.ReplyTo = tc.replyTo

			data, err := xml.Marshal(&ctrl)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			if tc.replyTo == "" {
				if strings.Contains(string(data), "replyTo") {
					t.Errorf("empty ReplyTo should be omitted via omitempty; got XML=%s", string(data))
				}
				return
			}

			// Assert the serialized bytes, not the round trip: the round
			// trip below passes under the element encoding too.
			if !strings.Contains(string(data), ` replyTo="`+tc.replyTo+`"`) {
				t.Errorf("replyTo must be an ATTRIBUTE (sep.xsd:5435); got XML=%s", string(data))
			}
			if strings.Contains(string(data), "<replyTo>") {
				t.Errorf("replyTo emitted as a child element; got XML=%s", string(data))
			}

			var decoded sep2.DERControl
			if err := xml.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if decoded.ReplyTo != tc.replyTo {
				t.Errorf("ReplyTo round-trip = %q, want %q", decoded.ReplyTo, tc.replyTo)
			}
		})
	}
}

// TestEventResponseRequiredRoundTrip asserts the HexBinary8 bitmap
// (Table 32 : bits select which transition statuses require a
// Response POST) round-trips through xml.Marshal/Unmarshal. The
// OnTransition hook ANDs this mask against the transition status to
// decide whether to call PostResponse.
func TestEventResponseRequiredRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		mask sep2.HexBinary8
	}{
		{name: "all_bits_clear", mask: 0x00},
		{name: "bit0_received_only", mask: 0x01},
		{name: "bits0_1_2_received_started_completed", mask: 0x07},
		{name: "all_bits_set", mask: 0xFF},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mask := tc.mask
			ctrl := sep2.DERControl{}
			ctrl.MRID = "DERC-002"
			ctrl.ResponseRequired = &mask

			data, err := xml.Marshal(&ctrl)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			// The exact attribute text, not merely its presence. A
			// HexBinary8 in attribute position needs MarshalXMLAttr to
			// reach the wire as hex; without it encoding/xml would render
			// the same value in decimal and a hexBinary parser would read a
			// different number without erroring.
			wantAttr := ` responseRequired="` + hexBinary8Text(tc.mask) + `"`
			if !strings.Contains(string(data), wantAttr) {
				t.Errorf("responseRequired must be an ATTRIBUTE (sep.xsd:5440) with value %s; got XML=%s",
					wantAttr, string(data))
			}
			if strings.Contains(string(data), "<responseRequired>") {
				t.Errorf("responseRequired emitted as a child element; got XML=%s", string(data))
			}

			var decoded sep2.DERControl
			if err := xml.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if decoded.ResponseRequired == nil {
				t.Fatalf("ResponseRequired nil after round-trip")
			}
			if *decoded.ResponseRequired != tc.mask {
				t.Errorf("ResponseRequired round-trip = %#x, want %#x", *decoded.ResponseRequired, tc.mask)
			}
		})
	}
}

// TestEventResponseRequiredOmitEmpty verifies that a DERControl with
// no ResponseRequired pointer set marshals WITHOUT a <responseRequired>
// element. This guards the omitempty contract : pre-2023 servers and
// clients must not see an unknown element.
func TestEventResponseRequiredOmitEmpty(t *testing.T) {
	t.Parallel()

	ctrl := sep2.DERControl{}
	ctrl.MRID = "DERC-003"

	data, err := xml.Marshal(&ctrl)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "responseRequired") {
		t.Errorf("responseRequired should be omitted when nil; got XML=%s", string(data))
	}
}

// hexBinary8Text renders the canonical hexBinary lexical form the wire
// expects for a HexBinary8: two uppercase hex digits.
func hexBinary8Text(v sep2.HexBinary8) string {
	return fmt.Sprintf("%02X", uint8(v))
}

// TestEventElementOrder asserts the XSD-required child element order (mRID
// before creationTime before EventStatus) and, separately, that replyTo and
// responseRequired appear on the START TAG rather than among the children.
//
// This test used to assert an ordering among `<replyTo>` and
// `<responseRequired>` elements, which encoded a defect as the expectation:
// the test could only pass while the two fields were wrongly modelled. An
// attribute has no position in the xsd:sequence, so the question the old
// assertion asked was not a real one.
func TestEventElementOrder(t *testing.T) {
	t.Parallel()

	mask := sep2.HexBinary8(0x07)
	ctrl := sep2.DERControl{}
	ctrl.MRID = "DERC-ORDER"
	ctrl.ReplyTo = "/rsp"
	ctrl.ResponseRequired = &mask
	ctrl.CreationTime = 1000
	ctrl.EventStatus = &sep2.EventStatus{CurrentStatus: 1, DateTime: 1000}

	data, err := xml.Marshal(&ctrl)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	xmlStr := string(data)

	// The two respondable fields belong on the start tag. Bounding the
	// search by the end of the start tag is what makes this an assertion
	// about attribute position and not merely about substring presence.
	endOfStartTag := strings.Index(xmlStr, ">")
	if endOfStartTag < 0 {
		t.Fatalf("no start tag in marshalled XML: %s", xmlStr)
	}
	startTag := xmlStr[:endOfStartTag]
	for _, attr := range []string{`replyTo="/rsp"`, `responseRequired="07"`} {
		if !strings.Contains(startTag, attr) {
			t.Errorf("%s must appear on the DERControl start tag; start tag was %q\nXML=%s",
				attr, startTag, xmlStr)
		}
	}

	posMRID := strings.Index(xmlStr, "<mRID>")
	posCreation := strings.Index(xmlStr, "<creationTime>")
	posEvtStatus := strings.Index(xmlStr, "<EventStatus>")

	if posMRID < 0 || posCreation < 0 || posEvtStatus < 0 {
		t.Fatalf("missing element(s): mRID=%d creationTime=%d EventStatus=%d\nXML=%s",
			posMRID, posCreation, posEvtStatus, xmlStr)
	}
	if !(posMRID < posCreation && posCreation < posEvtStatus) {
		t.Errorf("XSD element order violated: mRID=%d creationTime=%d EventStatus=%d (want strictly increasing)\nXML=%s",
			posMRID, posCreation, posEvtStatus, xmlStr)
	}
}

// TestDERControlCopyResponseRequiredDeepCopy asserts that DERControl.Copy
// produces an independent ResponseRequired pointer: mutating the copy
// must not change the original. The OnTransition hook may stash a copy of
// the active DERControl and mutate the bitmap during retry-policy
// evaluation; without deep-copy, the canonical event in the store
// would silently change.
func TestDERControlCopyResponseRequiredDeepCopy(t *testing.T) {
	t.Parallel()

	mask := sep2.HexBinary8(0x07)
	ctrl := sep2.DERControl{}
	ctrl.MRID = "DERC-COPY"
	ctrl.ResponseRequired = &mask

	copied := ctrl.Copy()
	if copied.ResponseRequired == nil {
		t.Fatalf("copy ResponseRequired = nil, want non-nil clone")
	}
	if copied.ResponseRequired == ctrl.ResponseRequired {
		t.Errorf("copy ResponseRequired shares pointer with original; want independent allocation")
	}
	*copied.ResponseRequired = 0xFF
	if *ctrl.ResponseRequired != 0x07 {
		t.Errorf("original ResponseRequired mutated to %#x after copy mutation; deep-copy violated", *ctrl.ResponseRequired)
	}
}

// TestEndDeviceControlCopyResponseRequiredDeepCopy asserts the same
// deep-copy contract for the DRLC EndDeviceControl subtype.
func TestEndDeviceControlCopyResponseRequiredDeepCopy(t *testing.T) {
	t.Parallel()

	mask := sep2.HexBinary8(0x03)
	ctrl := sep2.EndDeviceControl{}
	ctrl.MRID = "EDC-COPY"
	ctrl.ResponseRequired = &mask

	copied := ctrl.Copy()
	if copied.ResponseRequired == nil {
		t.Fatalf("copy ResponseRequired = nil, want non-nil clone")
	}
	if copied.ResponseRequired == ctrl.ResponseRequired {
		t.Errorf("copy ResponseRequired shares pointer with original")
	}
	*copied.ResponseRequired = 0xFF
	if *ctrl.ResponseRequired != 0x03 {
		t.Errorf("original mutated to %#x", *ctrl.ResponseRequired)
	}
}

// TestTextMessageCopyResponseRequiredDeepCopy asserts deep-copy on the
// Messaging function set's TextMessage subtype.
func TestTextMessageCopyResponseRequiredDeepCopy(t *testing.T) {
	t.Parallel()

	mask := sep2.HexBinary8(0x05)
	tm := sep2.TextMessage{}
	tm.MRID = "TM-COPY"
	tm.ResponseRequired = &mask

	copied := tm.Copy()
	if copied.ResponseRequired == nil {
		t.Fatalf("copy ResponseRequired = nil")
	}
	if copied.ResponseRequired == tm.ResponseRequired {
		t.Errorf("copy ResponseRequired shares pointer with original")
	}
	*copied.ResponseRequired = 0xFF
	if *tm.ResponseRequired != 0x05 {
		t.Errorf("original mutated to %#x", *tm.ResponseRequired)
	}
}

// TestFlowReservationResponseCopyResponseRequiredDeepCopy asserts
// deep-copy on the Flow Reservation function set's
// FlowReservationResponse subtype.
func TestFlowReservationResponseCopyResponseRequiredDeepCopy(t *testing.T) {
	t.Parallel()

	mask := sep2.HexBinary8(0x01)
	frp := sep2.FlowReservationResponse{}
	frp.MRID = "FRP-COPY"
	frp.ResponseRequired = &mask

	copied := frp.Copy()
	if copied.ResponseRequired == nil {
		t.Fatalf("copy ResponseRequired = nil")
	}
	if copied.ResponseRequired == frp.ResponseRequired {
		t.Errorf("copy ResponseRequired shares pointer with original")
	}
	*copied.ResponseRequired = 0xFF
	if *frp.ResponseRequired != 0x01 {
		t.Errorf("original mutated to %#x", *frp.ResponseRequired)
	}
}
