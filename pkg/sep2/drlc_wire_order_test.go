package sep2_test

import (
	"encoding/xml"
	"reflect"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/internal/xsdgate"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

func fullDemandResponseProgram() sep2.DemandResponseProgram {
	pct := sep2.PerCent(2500)
	return sep2.DemandResponseProgram{
		XMLName:                                  xml.Name{Space: "urn:ieee:std:2030.5:ns", Local: "DemandResponseProgram"},
		Resource:                                 sep2.Resource{Href: "/dr/1"},
		IdentifiedObject:                         sep2.IdentifiedObject{MRID: "0F", Description: "prog"},
		ActiveEndDeviceControlListLink:           &sep2.ListLink{Href: "/dr/1/actedc", All: 1},
		AvailabilityUpdatePercentChangeThreshold: &pct,
		AvailabilityUpdatePowerChangeThreshold:   &sep2.ActivePower{Multiplier: 3, Value: 5},
		EndDeviceControlListLink:                 &sep2.ListLink{Href: "/dr/1/edc", All: 2},
		Primacy:                                  7,
	}
}

// TestDemandResponseProgramWireOrder asserts the sep.xsd sequence for
// DemandResponseProgram: IdentifiedObject's elements, then the active list
// link, the two availability thresholds, the control list link, primacy.
func TestDemandResponseProgramWireOrder(t *testing.T) {
	drp := fullDemandResponseProgram()
	data, err := xml.Marshal(&drp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	assertOrder(t, string(data), []string{
		"<mRID>",
		"<description>",
		"<ActiveEndDeviceControlListLink",
		"<availabilityUpdatePercentChangeThreshold>2500<",
		"<availabilityUpdatePowerChangeThreshold><multiplier>3</multiplier><value>5</value>",
		"<EndDeviceControlListLink",
		"<primacy>7</primacy>",
	})
}

func TestDemandResponseProgramRoundTrip(t *testing.T) {
	drp := fullDemandResponseProgram()
	data, err := xml.Marshal(&drp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got sep2.DemandResponseProgram
	if err := xml.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(got, drp) {
		t.Errorf("round trip mismatch\n got: %+v\nwant: %+v\nxml: %s", got, drp, data)
	}
	if *got.AvailabilityUpdatePercentChangeThreshold != 2500 {
		t.Errorf("percent threshold = %d, want 2500", *got.AvailabilityUpdatePercentChangeThreshold)
	}
	if *got.AvailabilityUpdatePowerChangeThreshold != (sep2.ActivePower{Multiplier: 3, Value: 5}) {
		t.Errorf("power threshold = %+v", *got.AvailabilityUpdatePowerChangeThreshold)
	}
}

func TestDemandResponseProgramCopyThresholds(t *testing.T) {
	drp := fullDemandResponseProgram()
	c := drp.Copy()
	*c.AvailabilityUpdatePercentChangeThreshold = 1
	c.AvailabilityUpdatePowerChangeThreshold.Value = 99
	if *drp.AvailabilityUpdatePercentChangeThreshold != 2500 {
		t.Error("percent threshold shared with copy")
	}
	if drp.AvailabilityUpdatePowerChangeThreshold.Value != 5 {
		t.Error("power threshold shared with copy")
	}
}

// TestDemandResponseProgramPopulatedSchemaValid validates a fully populated
// program against sep.xsd (skips only when the schema is not provisioned).
func TestDemandResponseProgramPopulatedSchemaValid(t *testing.T) {
	xsdgate.AssertValid(t, "DemandResponseProgram", fullDemandResponseProgram())
}
