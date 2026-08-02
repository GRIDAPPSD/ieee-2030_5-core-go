package sep2_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// TestDERStatusStateOfChargeStatusUnmarshal is the IEEECORE-055 regression at
// the type level. sep.xsd:4189 declares stateOfChargeStatus as
// StateOfChargeStatusType, and sep.xsd:4566-4582 declares that type as a
// sequence of a required dateTime (TimeType) and a required value (PerCent).
// Modelling it as a bare *uint16 made encoding/xml try to parse the element's
// chardata as an unsigned integer, which failed on a spec-correct client's
// nested form.
func TestDERStatusStateOfChargeStatusUnmarshal(t *testing.T) {
	t.Parallel()

	const body = `<DERStatus xmlns="urn:ieee:std:2030.5:ns">
  <readingTime>1604963587</readingTime>
  <stateOfChargeStatus>
    <dateTime>1604963587</dateTime>
    <value>7500</value>
  </stateOfChargeStatus>
  <storageModeStatus>
    <dateTime>1604963588</dateTime>
    <value>1</value>
  </storageModeStatus>
</DERStatus>`

	var got sep2.DERStatus
	if err := xml.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("unmarshal spec-correct DERStatus: %v", err)
	}

	if got.StateOfChargeStatus == nil {
		t.Fatal("stateOfChargeStatus is nil")
	}
	if want := int64(1604963587); got.StateOfChargeStatus.DateTime != want {
		t.Errorf("stateOfChargeStatus.dateTime = %d, want %d", got.StateOfChargeStatus.DateTime, want)
	}
	// PerCent is hundredths of a percent (sep.xsd:5945-5952), so 7500 is 75%
	// and the field must be wide enough to hold the full 0 to 10000 range.
	if want := uint16(7500); got.StateOfChargeStatus.Value != want {
		t.Errorf("stateOfChargeStatus.value = %d, want %d", got.StateOfChargeStatus.Value, want)
	}

	if got.StorageModeStatus == nil {
		t.Fatal("storageModeStatus is nil")
	}
	if want := int64(1604963588); got.StorageModeStatus.DateTime != want {
		t.Errorf("storageModeStatus.dateTime = %d, want %d", got.StorageModeStatus.DateTime, want)
	}
	if want := uint8(1); got.StorageModeStatus.Value != want {
		t.Errorf("storageModeStatus.value = %d, want %d", got.StorageModeStatus.Value, want)
	}
}

// TestDERStatusStateOfChargeStatusPerCentRange asserts the full PerCent range
// survives a round trip. A uint8 field would wrap 10000 to 16; asserting the
// boundary value is what makes that failure visible rather than silent.
func TestDERStatusStateOfChargeStatusPerCentRange(t *testing.T) {
	t.Parallel()

	for _, pct := range []uint16{0, 1, 5000, 9999, 10000} {
		status := sep2.DERStatus{
			ReadingTime:         1604963587,
			StateOfChargeStatus: &sep2.StateOfChargeStatusType{DateTime: 1604963587, Value: pct},
		}
		data, err := xml.Marshal(&status)
		if err != nil {
			t.Fatalf("marshal %d: %v", pct, err)
		}

		var got sep2.DERStatus
		if err := xml.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal %d: %v", pct, err)
		}
		if got.StateOfChargeStatus == nil {
			t.Fatalf("%d: stateOfChargeStatus is nil after round trip", pct)
		}
		if got.StateOfChargeStatus.Value != pct {
			t.Errorf("round trip of PerCent %d gave %d", pct, got.StateOfChargeStatus.Value)
		}
	}
}

// TestDERStatusStateOfChargeStatusMarshal asserts we EMIT the nested form,
// not just parse it. Fixing only the unmarshal path would still hand the
// client a document it must reject on GET.
func TestDERStatusStateOfChargeStatusMarshal(t *testing.T) {
	t.Parallel()

	status := sep2.DERStatus{
		ReadingTime:         1604963587,
		StateOfChargeStatus: &sep2.StateOfChargeStatusType{DateTime: 1604963587, Value: 7500},
		StorageModeStatus:   &sep2.StorageModeStatusType{DateTime: 1604963588, Value: 2},
	}

	data, err := xml.Marshal(&status)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(data)

	for _, want := range []string{
		"<stateOfChargeStatus><dateTime>1604963587</dateTime><value>7500</value></stateOfChargeStatus>",
		"<storageModeStatus><dateTime>1604963588</dateTime><value>2</value></storageModeStatus>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("marshalled output missing %q\ngot: %s", want, out)
		}
	}
}

// TestDERStatusOptionalStatusesOmitted asserts both elements stay omissible.
// Both are minOccurs="0" (sep.xsd:4189, sep.xsd:4195), and a struct value
// rather than a pointer would emit an empty element carrying a dateTime of 0,
// which asserts a state that was never reported.
func TestDERStatusOptionalStatusesOmitted(t *testing.T) {
	t.Parallel()

	data, err := xml.Marshal(&sep2.DERStatus{ReadingTime: 1604963587})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(data)

	for _, absent := range []string{"stateOfChargeStatus", "storageModeStatus"} {
		if strings.Contains(out, absent) {
			t.Errorf("optional element %q emitted when unset\ngot: %s", absent, out)
		}
	}
}
