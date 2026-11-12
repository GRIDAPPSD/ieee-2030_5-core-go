package sep2_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/internal/xsdgate"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

const drResponseBody = `<DrResponse xmlns="urn:ieee:std:2030.5:ns" href="/rsps/1/rsp/2">` +
	`<createdDateTime>1785429793</createdDateTime>` +
	`<endDeviceLFDI>AABBCCDDEEFF00112233445566778899AABBCCDD</endDeviceLFDI>` +
	`<status>8</status>` +
	`<subject>0123456789ABCDEF0123456789ABCDEF</subject>` +
	`<ApplianceLoadReduction><type>1</type></ApplianceLoadReduction>` +
	`<AppliedTargetReduction><type>2</type><value>12</value></AppliedTargetReduction>` +
	`<DutyCycle><normalValue>30</normalValue></DutyCycle>` +
	`<Offset><coolingOffset>5</coolingOffset><heatingOffset>6</heatingOffset>` +
	`<loadAdjustmentPercentageOffset>2500</loadAdjustmentPercentageOffset></Offset>` +
	`<overrideDuration>600</overrideDuration>` +
	`<SetPoint><coolingSetpoint>-2150</coolingSetpoint><heatingSetpoint>2100</heatingSetpoint></SetPoint>` +
	`</DrResponse>`

func TestDecodeResponse_DrResponseCarriesDRLCValues(t *testing.T) {
	t.Parallel()

	got, err := sep2.DecodeResponse([]byte(drResponseBody))
	if err != nil {
		t.Fatalf("DecodeResponse(<DrResponse>) = %v, want it accepted", err)
	}
	if got.Href != "/rsps/1/rsp/2" {
		t.Errorf("Href = %q, want /rsps/1/rsp/2", got.Href)
	}
	if got.EndDeviceLFDI != "AABBCCDDEEFF00112233445566778899AABBCCDD" ||
		got.Subject != "0123456789ABCDEF0123456789ABCDEF" || got.CreatedDateTime != 1785429793 ||
		got.Status == nil || *got.Status != 8 {
		t.Errorf("base Response fields = %+v, want the POSTed values", got.Response)
	}
	d := got.DRLC
	if d == nil {
		t.Fatal("DRLC is nil, want the six DRLC children")
	}
	if d.ApplianceLoadReduction == nil || d.ApplianceLoadReduction.Type != 1 {
		t.Errorf("ApplianceLoadReduction = %+v, want type 1", d.ApplianceLoadReduction)
	}
	if d.AppliedTargetReduction == nil || d.AppliedTargetReduction.Type != 2 || d.AppliedTargetReduction.Value != 12 {
		t.Errorf("AppliedTargetReduction = %+v, want type 2 value 12", d.AppliedTargetReduction)
	}
	if d.DutyCycle == nil || d.DutyCycle.NormalValue != 30 {
		t.Errorf("DutyCycle = %+v, want 30", d.DutyCycle)
	}
	if d.Offset == nil || d.Offset.CoolingOffset == nil || *d.Offset.CoolingOffset != 5 ||
		d.Offset.HeatingOffset == nil || *d.Offset.HeatingOffset != 6 ||
		d.Offset.LoadAdjustmentPercentageOffset == nil || *d.Offset.LoadAdjustmentPercentageOffset != 2500 {
		t.Errorf("Offset = %+v, want 5, 6, 2500", d.Offset)
	}
	if d.OverrideDuration == nil || *d.OverrideDuration != 600 {
		t.Errorf("OverrideDuration = %v, want 600", d.OverrideDuration)
	}
	if d.SetPoint == nil || d.SetPoint.CoolingSetpoint == nil || *d.SetPoint.CoolingSetpoint != -2150 ||
		d.SetPoint.HeatingSetpoint == nil || *d.SetPoint.HeatingSetpoint != 2100 {
		t.Errorf("SetPoint = %+v, want -2150, 2100", d.SetPoint)
	}
}

// TestDecodeResponse_EveryRootClearsXMLName fails if the clear in
// DecodeResponse is removed: a <Response> root fills Response.XMLName, so a
// surviving value shows. A DrResponse root never fills the embedded one, so
// it is checked on the wire as well: it marshals as a bare <Response>.
func TestDecodeResponse_EveryRootClearsXMLName(t *testing.T) {
	t.Parallel()
	for _, root := range sep2.ResponseRootElements {
		got, err := sep2.DecodeResponse([]byte(responseBody(root, sep2.Namespace)))
		if err != nil {
			t.Fatalf("DecodeResponse(<%s>): %v", root, err)
		}
		if got.XMLName != (xml.Name{}) {
			t.Errorf("<%s>: XMLName = %v, want it cleared", root, got.XMLName)
		}
		b, err := xml.Marshal(&got.Response)
		if err != nil {
			t.Fatalf("<%s>: marshal: %v", root, err)
		}
		if !strings.HasPrefix(string(b), "<Response ") {
			t.Errorf("<%s>: decoded Response marshals as %s, want a <Response> root", root, b)
		}
	}
}

func TestDecodeResponse_OtherSubtypesCarryNoDRLC(t *testing.T) {
	t.Parallel()
	for _, root := range sep2.ResponseRootElements {
		if root == "DrResponse" {
			continue
		}
		got, err := sep2.DecodeResponse([]byte(responseBody(root, sep2.Namespace)))
		if err != nil {
			t.Fatalf("DecodeResponse(<%s>): %v", root, err)
		}
		if got.DRLC != nil {
			t.Errorf("<%s> decoded with DRLC %+v, want nil", root, got.DRLC)
		}
	}
}

func TestDecodeResponse_DrResponseWithoutChildren(t *testing.T) {
	t.Parallel()
	got, err := sep2.DecodeResponse([]byte(responseBody("DrResponse", sep2.Namespace)))
	if err != nil {
		t.Fatalf("DecodeResponse: %v", err)
	}
	if got.DRLC == nil {
		t.Fatal("DRLC is nil, want a non-nil value with every child absent")
	}
	d := got.DRLC
	if d.ApplianceLoadReduction != nil || d.AppliedTargetReduction != nil || d.DutyCycle != nil ||
		d.Offset != nil || d.OverrideDuration != nil || d.SetPoint != nil {
		t.Errorf("DRLC = %+v, want all six children absent", d)
	}
}

func TestDecodeResponse_DrResponseForeignNamespaceRefused(t *testing.T) {
	t.Parallel()
	body := strings.Replace(drResponseBody, sep2.Namespace, "urn:example:other", 1)
	if _, err := sep2.DecodeResponse([]byte(body)); err == nil {
		t.Fatal("DecodeResponse(foreign-namespace DrResponse) = nil error, want refused")
	}
}

func TestResponseRootElementsListsDrResponse(t *testing.T) {
	t.Parallel()
	for _, r := range sep2.ResponseRootElements {
		if r == "DrResponse" {
			return
		}
	}
	t.Errorf("ResponseRootElements = %v, want DrResponse listed", sep2.ResponseRootElements)
}

func TestDrResponseSchemaGateAndCopy(t *testing.T) {
	var d sep2.DrResponse
	if err := xml.Unmarshal([]byte(drResponseBody), &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	xsdgate.AssertValid(t, "DrResponse", d)

	c := d.Copy()
	c.Response.Subject = "x"
	*c.Response.Status = 1
	c.ApplianceLoadReduction.Type = 9
	c.AppliedTargetReduction.Value = 9
	c.DutyCycle.NormalValue = 9
	*c.Offset.CoolingOffset = 9
	*c.OverrideDuration = 1
	*c.SetPoint.CoolingSetpoint = 9
	if d.Subject != "0123456789ABCDEF0123456789ABCDEF" || *d.Status != 8 || d.ApplianceLoadReduction.Type != 1 ||
		d.AppliedTargetReduction.Value != 12 || d.DutyCycle.NormalValue != 30 || *d.Offset.CoolingOffset != 5 ||
		*d.OverrideDuration != 600 || *d.SetPoint.CoolingSetpoint != -2150 {
		t.Errorf("mutating the copy changed the original: %+v", d)
	}
}

func TestDecodeResponse_AppliedTargetReductionRefusesMissingChild(t *testing.T) {
	t.Parallel()
	for name, child := range map[string]string{
		"no value": `<type>2</type>`,
		"no type":  `<value>12</value>`,
		"empty":    ``,
	} {
		body := `<DrResponse xmlns="urn:ieee:std:2030.5:ns"><AppliedTargetReduction>` + child + `</AppliedTargetReduction></DrResponse>`
		if _, err := sep2.DecodeResponse([]byte(body)); err == nil {
			t.Errorf("%s: DecodeResponse = nil error, want a missing required child refused", name)
		}
	}
}

func TestDecodeResponse_AppliedTargetReductionRefusesReservedUnitType(t *testing.T) {
	t.Parallel()
	body := `<DrResponse xmlns="urn:ieee:std:2030.5:ns"><AppliedTargetReduction><type>13</type><value>12</value></AppliedTargetReduction></DrResponse>`
	if _, err := sep2.DecodeResponse([]byte(body)); err == nil {
		t.Error("DecodeResponse = nil error, want a reserved UnitType refused")
	}
}
