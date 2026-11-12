package sep2_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/internal/xsdgate"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// edcBody carries every EndDeviceControl element in XSD sequence order.
const edcBody = `<EndDeviceControl xmlns="urn:ieee:std:2030.5:ns" href="/drp/0/edc/1">` +
	`<mRID>0102030405060708090A0B0C0D0E0F10</mRID>` +
	`<creationTime>1500000000</creationTime>` +
	`<EventStatus><currentStatus>1</currentStatus><dateTime>1500000000</dateTime></EventStatus>` +
	`<interval><duration>3600</duration><start>1500000000</start></interval>` +
	`<ApplianceLoadReduction><type>1</type></ApplianceLoadReduction>` +
	`<deviceCategory>40</deviceCategory>` +
	`<drProgramMandatory>true</drProgramMandatory>` +
	`<DutyCycle><normalValue>30</normalValue></DutyCycle>` +
	`<loadShiftForward>true</loadShiftForward>` +
	`<Offset><coolingOffset>5</coolingOffset><heatingOffset>6</heatingOffset>` +
	`<loadAdjustmentPercentageOffset>2500</loadAdjustmentPercentageOffset></Offset>` +
	`<overrideDuration>600</overrideDuration>` +
	`<SetPoint><coolingSetpoint>-2150</coolingSetpoint><heatingSetpoint>2100</heatingSetpoint></SetPoint>` +
	`<TargetReduction><type>0</type><value>12</value></TargetReduction>` +
	`</EndDeviceControl>`

// TestEndDeviceControlDecodesEveryElement reads each schema element back
// from a decoded body, so a dropped element fails by value.
func TestEndDeviceControlDecodesEveryElement(t *testing.T) {
	var e sep2.EndDeviceControl
	if err := xml.Unmarshal([]byte(edcBody), &e); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if e.DeviceCategory == nil || *e.DeviceCategory != 0x40 {
		t.Errorf("DeviceCategory = %v, want 0x40", e.DeviceCategory)
	}
	if !e.DRProgramMandatory || !e.LoadShiftForward {
		t.Errorf("booleans = %v %v, want true true", e.DRProgramMandatory, e.LoadShiftForward)
	}
	if e.OverrideDuration == nil || *e.OverrideDuration != 600 {
		t.Errorf("OverrideDuration = %v, want 600", e.OverrideDuration)
	}
	if e.ApplianceLoadReduction == nil || e.ApplianceLoadReduction.Type != 1 {
		t.Errorf("ApplianceLoadReduction = %+v, want type 1", e.ApplianceLoadReduction)
	}
	if e.DutyCycle == nil || e.DutyCycle.NormalValue != 30 {
		t.Errorf("DutyCycle = %+v, want 30", e.DutyCycle)
	}
	o := e.Offset
	if o == nil || o.CoolingOffset == nil || *o.CoolingOffset != 5 ||
		o.HeatingOffset == nil || *o.HeatingOffset != 6 ||
		o.LoadAdjustmentPercentageOffset == nil || *o.LoadAdjustmentPercentageOffset != 2500 {
		t.Errorf("Offset = %+v, want 5, 6, 2500", o)
	}
	s := e.SetPoint
	if s == nil || s.CoolingSetpoint == nil || *s.CoolingSetpoint != -2150 ||
		s.HeatingSetpoint == nil || *s.HeatingSetpoint != 2100 {
		t.Errorf("SetPoint = %+v, want -2150, 2100", s)
	}
	if e.TargetReduction == nil || e.TargetReduction.Type != 0 || e.TargetReduction.Value != 12 {
		t.Errorf("TargetReduction = %+v, want type 0 value 12", e.TargetReduction)
	}
}

// TestEndDeviceControlReencodesInXSDOrder pins the child element order of the
// re-encoded bytes and that a second decode and encode is byte-stable.
func TestEndDeviceControlReencodesInXSDOrder(t *testing.T) {
	var e sep2.EndDeviceControl
	if err := xml.Unmarshal([]byte(edcBody), &e); err != nil {
		t.Fatalf("decode: %v", err)
	}
	out, err := xml.Marshal(e)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got := string(out)
	order := []string{
		"<interval>", "<ApplianceLoadReduction>", "<deviceCategory>", "<drProgramMandatory>",
		"<DutyCycle>", "<loadShiftForward>", "<Offset>", "<overrideDuration>",
		"<SetPoint>", "<TargetReduction>",
	}
	prev := -1
	for _, el := range order {
		i := strings.Index(got, el)
		if i < 0 {
			t.Errorf("re-encoded body lacks %s:\n%s", el, got)
			continue
		}
		if i < prev {
			t.Errorf("%s out of XSD order in:\n%s", el, got)
		}
		prev = i
	}
	for _, want := range []string{
		"<drProgramMandatory>true</drProgramMandatory>",
		"<loadShiftForward>true</loadShiftForward>",
		"<overrideDuration>600</overrideDuration>",
		"<deviceCategory>40</deviceCategory>",
		"<ApplianceLoadReduction><type>1</type></ApplianceLoadReduction>",
		"<DutyCycle><normalValue>30</normalValue></DutyCycle>",
		"<coolingSetpoint>-2150</coolingSetpoint>",
		"<loadAdjustmentPercentageOffset>2500</loadAdjustmentPercentageOffset>",
		"<TargetReduction><type>0</type><value>12</value></TargetReduction>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("re-encoded body lacks %s:\n%s", want, got)
		}
	}
	var again sep2.EndDeviceControl
	if err := xml.Unmarshal(out, &again); err != nil {
		t.Fatalf("second decode: %v", err)
	}
	out2, err := xml.Marshal(again)
	if err != nil || string(out2) != got {
		t.Errorf("second round trip differs (err %v):\n%s\n%s", err, got, out2)
	}
}

// TestEndDeviceControlMissingBooleansReadAsZero: a body without the two
// required booleans decodes to false and is not given a value.
func TestEndDeviceControlMissingBooleansReadAsZero(t *testing.T) {
	var e sep2.EndDeviceControl
	body := `<EndDeviceControl xmlns="urn:ieee:std:2030.5:ns"><deviceCategory>40</deviceCategory></EndDeviceControl>`
	if err := xml.Unmarshal([]byte(body), &e); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if e.DRProgramMandatory || e.LoadShiftForward {
		t.Errorf("missing booleans read as %v %v, want false false", e.DRProgramMandatory, e.LoadShiftForward)
	}
	if e.OverrideDuration != nil || e.Offset != nil || e.SetPoint != nil ||
		e.TargetReduction != nil || e.DutyCycle != nil || e.ApplianceLoadReduction != nil {
		t.Errorf("absent optional elements were invented: %+v", e)
	}
}

// TestEndDeviceControlRequiredBooleansAlwaysEncoded: false is on the wire,
// because both elements are minOccurs 1.
func TestEndDeviceControlRequiredBooleansAlwaysEncoded(t *testing.T) {
	out, err := xml.Marshal(sep2.EndDeviceControl{})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	for _, want := range []string{"<drProgramMandatory>false</drProgramMandatory>", "<loadShiftForward>false</loadShiftForward>"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("encoded zero value lacks %s:\n%s", want, out)
		}
	}
}

// TestEndDeviceControlSchemaGate validates a fully populated control, and its
// definition, against sep.xsd; sub-type integer widths are checked by the
// struct lane.
func TestEndDeviceControlSchemaGate(t *testing.T) {
	var e sep2.EndDeviceControl
	if err := xml.Unmarshal([]byte(edcBody), &e); err != nil {
		t.Fatalf("decode: %v", err)
	}
	xsdgate.AssertValid(t, "EndDeviceControl", e)
	// EndDeviceControl's own struct lane carries older omitempty findings on
	// base fields; the integer lane is pinned in TestSchemaGateControlIntegerPins.
	xsdgate.AssertStructValid(t, "ApplianceLoadReduction", sep2.ApplianceLoadReduction{})
	xsdgate.AssertStructValid(t, "DutyCycle", sep2.DutyCycle{})
	xsdgate.AssertStructValid(t, "Offset", sep2.Offset{})
	xsdgate.AssertStructValid(t, "SetPoint", sep2.SetPoint{})
	xsdgate.AssertStructValid(t, "TargetReduction", sep2.TargetReduction{})
}

// TestEndDeviceControlCopyIndependent mutates every new pointer on a copy and
// reads the original back.
func TestEndDeviceControlCopyIndependent(t *testing.T) {
	var e sep2.EndDeviceControl
	if err := xml.Unmarshal([]byte(edcBody), &e); err != nil {
		t.Fatalf("decode: %v", err)
	}
	c := e.Copy()
	*c.OverrideDuration = 1
	c.ApplianceLoadReduction.Type = 9
	c.DutyCycle.NormalValue = 9
	*c.Offset.CoolingOffset = 9
	*c.Offset.HeatingOffset = 9
	*c.Offset.LoadAdjustmentPercentageOffset = 9
	*c.SetPoint.CoolingSetpoint = 9
	*c.SetPoint.HeatingSetpoint = 9
	c.TargetReduction.Value = 9
	if *e.OverrideDuration != 600 || e.ApplianceLoadReduction.Type != 1 || e.DutyCycle.NormalValue != 30 ||
		*e.Offset.CoolingOffset != 5 || *e.Offset.HeatingOffset != 6 || *e.Offset.LoadAdjustmentPercentageOffset != 2500 ||
		*e.SetPoint.CoolingSetpoint != -2150 || *e.SetPoint.HeatingSetpoint != 2100 || e.TargetReduction.Value != 12 {
		t.Errorf("mutating the copy changed the original: %+v", e)
	}
}

// TestEndDeviceControlOutOfRangeRejected: Offset's percentage keeps PerCent's
// range check on decode.
func TestEndDeviceControlOutOfRangeRejected(t *testing.T) {
	body := `<EndDeviceControl xmlns="urn:ieee:std:2030.5:ns"><Offset><loadAdjustmentPercentageOffset>10001</loadAdjustmentPercentageOffset></Offset></EndDeviceControl>`
	var e sep2.EndDeviceControl
	if err := xml.Unmarshal([]byte(body), &e); err == nil {
		t.Errorf("out-of-range loadAdjustmentPercentageOffset decoded: %+v", e.Offset)
	}
}
