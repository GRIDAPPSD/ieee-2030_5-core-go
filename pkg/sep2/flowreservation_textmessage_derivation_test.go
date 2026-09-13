package sep2_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// TestFlowReservationResponseOmitsRandomizeFields pins #107:
// sep.xsd derives FlowReservationResponse from Event, not RandomizableEvent,
// so randomizeStart/randomizeDuration must never reach the wire for it.
func TestFlowReservationResponseOmitsRandomizeFields(t *testing.T) {
	f := sep2.FlowReservationResponse{
		Event: sep2.Event{CreationTime: 1500000000},
	}
	data, err := xml.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	xmlStr := string(data)
	for _, forbidden := range []string{"randomizeStart", "randomizeDuration"} {
		if strings.Contains(xmlStr, forbidden) {
			t.Errorf("FlowReservationResponse must not emit %s; xml:\n%s", forbidden, xmlStr)
		}
	}
}

// TestTextMessageOmitsRandomizeFields pins #107:
// sep.xsd derives TextMessage from Event, not RandomizableEvent.
func TestTextMessageOmitsRandomizeFields(t *testing.T) {
	tm := sep2.TextMessage{
		Event: sep2.Event{CreationTime: 1500000000},
	}
	data, err := xml.Marshal(tm)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	xmlStr := string(data)
	for _, forbidden := range []string{"randomizeStart", "randomizeDuration"} {
		if strings.Contains(xmlStr, forbidden) {
			t.Errorf("TextMessage must not emit %s; xml:\n%s", forbidden, xmlStr)
		}
	}
}
