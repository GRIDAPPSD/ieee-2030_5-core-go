package sep2_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// TestEventCreationTimeIsAlwaysServed asserts that creationTime appears in
// the marshalled bytes even when it is zero.
//
// creationTime is declared minOccurs="1" on the XSD Event type, so it is not
// optional and must not carry omitempty. A schema-driven parser rejects the
// WHOLE document when a required element is absent (the EPRI reference
// client's xml_parse.c sets PARSE_INVALID on a missing required element), so
// an omitted creationTime does not degrade one field, it silently discards
// the entire DERControl.
//
// Asserting the ZERO case specifically is the point: an omitempty tag would
// still emit the element for every non-zero value, so a test that only sets
// a real timestamp would pass against the defect this pins.
func TestEventCreationTimeIsAlwaysServed(t *testing.T) {
	t.Parallel()

	ctrl := sep2.DERControl{}
	ctrl.MRID = "0123456789ABCDEF0123456789ABCDEF"

	data, err := xml.Marshal(&ctrl)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(data)

	if !strings.Contains(got, "<creationTime>0</creationTime>") {
		t.Errorf("marshalled DERControl with zero CreationTime omits the required <creationTime> element; a strict parser discards the whole resource:\n%s", got)
	}
}

// TestEventCreationTimeRoundTrips asserts a real creationTime survives an
// unmarshal of the served bytes with its value intact.
//
// The value, not merely the element, is what matters: a client decides which
// of two overlapping equal-primacy events supersedes the other by comparing
// creationTime (EPRI's block_supersede: x->creationTime > y->creationTime),
// so a creationTime that round-trips as the wrong number makes the wrong
// event win.
func TestEventCreationTimeRoundTrips(t *testing.T) {
	t.Parallel()

	const want int64 = 1785419217

	ctrl := sep2.DERControl{}
	ctrl.MRID = "0123456789ABCDEF0123456789ABCDEF"
	ctrl.CreationTime = want

	data, err := xml.Marshal(&ctrl)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var back sep2.DERControl
	if err := xml.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.CreationTime != want {
		t.Errorf("CreationTime round-tripped as %d, want %d\nbytes: %s", back.CreationTime, want, data)
	}
}

// TestEventWireOrderPlacesCreationTimeBeforeEventStatus asserts the XSD
// sequence position of creationTime on the served bytes.
//
// sep.xsd's Event sequence is creationTime, EventStatus, interval, and it
// extends RespondableSubscribableIdentifiedObject, whose own sequence
// contributes replyTo/responseRequired (attributes) then mRID, description,
// version. Order is significant: a schema-driven parser walks the sequence
// in declaration order and fails the document on an out-of-order element,
// so emitting creationTime after EventStatus would be as fatal as omitting
// it. encoding/xml marshals in struct field order, which is why this is
// pinned by a test rather than left to review.
func TestEventWireOrderPlacesCreationTimeBeforeEventStatus(t *testing.T) {
	t.Parallel()

	ctrl := sep2.DERControl{
		DERControlBase: &sep2.DERControlBase{},
	}
	ctrl.MRID = "0123456789ABCDEF0123456789ABCDEF"
	ctrl.Description = "test control"
	ctrl.CreationTime = 1785419217
	ctrl.EventStatus = &sep2.EventStatus{CurrentStatus: sep2.EventStatusActive}
	ctrl.Interval = &sep2.DateTimeInterval{Start: 1785419217, Duration: 900}

	data, err := xml.Marshal(&ctrl)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	assertOrder(t, string(data), []string{
		"<mRID>",
		"<description>",
		"<creationTime>",
		"<EventStatus>",
		"<interval>",
		"<DERControlBase>",
	})
}
