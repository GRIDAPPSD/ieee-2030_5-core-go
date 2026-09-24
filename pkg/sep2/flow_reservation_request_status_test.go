package sep2_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// TestFlowReservationRequestUnmarshalNestedRequestStatus is a regression test
// at the type level. sep.xsd declares RequestStatus as a complexType
// carrying dateTime and requestStatus, both minOccurs="1". Modelling it as a
// bare *uint8 made encoding/xml try to parse the element's chardata as an
// unsigned integer, which failed on a spec-correct client's nested form (#152).
func TestFlowReservationRequestUnmarshalNestedRequestStatus(t *testing.T) {
	t.Parallel()

	const body = `<FlowReservationRequest xmlns="urn:ieee:std:2030.5:ns">
  <creationTime>1604963587</creationTime>
  <RequestStatus>
    <dateTime>1604963588</dateTime>
    <requestStatus>1</requestStatus>
  </RequestStatus>
</FlowReservationRequest>`

	var got sep2.FlowReservationRequest
	if err := xml.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("unmarshal spec-correct FlowReservationRequest: %v", err)
	}

	if want := int64(1604963588); got.RequestStatus.DateTime != want {
		t.Errorf("RequestStatus.DateTime = %d, want %d", got.RequestStatus.DateTime, want)
	}
	if want := sep2.RequestStatusCancelled; got.RequestStatus.RequestStatus != want {
		t.Errorf("RequestStatus.RequestStatus = %d, want %d", got.RequestStatus.RequestStatus, want)
	}
}

// TestFlowReservationRequestRequestStatusMarshal asserts we EMIT the nested
// form, in the schema's element order (dateTime then requestStatus, both
// lowercase). Fixing only the unmarshal path would still hand the client a
// document it must reject.
func TestFlowReservationRequestRequestStatusMarshal(t *testing.T) {
	t.Parallel()

	req := sep2.FlowReservationRequest{
		CreationTime:  1604963587,
		RequestStatus: sep2.RequestStatus{DateTime: 1604963588, RequestStatus: sep2.RequestStatusRequested},
	}

	data, err := xml.Marshal(&req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(data)

	if want := "<RequestStatus><dateTime>1604963588</dateTime><requestStatus>0</requestStatus></RequestStatus>"; !strings.Contains(out, want) {
		t.Errorf("marshalled output missing %q\ngot: %s", want, out)
	}
}

// TestFlowReservationRequestRequestStatusAlwaysEmitted asserts the element
// always serializes, since it is minOccurs="1" on FlowReservationRequest: an
// unset RequestStatus is a zero value, not an absent element.
func TestFlowReservationRequestRequestStatusAlwaysEmitted(t *testing.T) {
	t.Parallel()

	data, err := xml.Marshal(&sep2.FlowReservationRequest{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(data)

	if want := "<RequestStatus>"; !strings.Contains(out, want) {
		t.Errorf("required element %q dropped from zero-value FlowReservationRequest\ngot: %s", want, out)
	}
}
