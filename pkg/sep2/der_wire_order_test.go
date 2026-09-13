package sep2_test

import (
	"encoding/xml"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// TestDERCapabilityWireOrder asserts DERCapability order against sep.xsd's
// DERCapability sequence: modesSupported, rtgAbnormalCategory, rtgMaxA,
// rtgMaxAh, rtgMaxChargeRateVA, rtgMaxChargeRateW, rtgMaxDischargeRateVA,
// rtgMaxDischargeRateW, rtgMaxV, rtgMaxVA, rtgMaxVar, rtgMaxVarNeg, rtgMaxW,
// ... type. The regression this guards: rtgMaxW was previously emitted
// second (right after modesSupported), far ahead of rtgMaxA/rtgMaxVar/etc,
// which a strict schema validator rejects.
func TestDERCapabilityWireOrder(t *testing.T) {
	modes := sep2.DERControlType(0xFF)
	maxA := int32(20)
	maxVar := sep2.ReactivePower{Value: 100}
	maxChargeW := sep2.ActivePower{Value: 200}
	maxDischargeW := sep2.ActivePower{Value: 300}
	maxW := sep2.ActivePower{Value: 5000}
	dtype := uint8(4)

	derCap := sep2.DERCapability{
		ModesSupported:       &modes,
		RTGMaxA:              &maxA,
		RTGMaxVar:            &maxVar,
		RTGMaxChargeRateW:    &maxChargeW,
		RTGMaxDischargeRateW: &maxDischargeW,
		RTGMaxW:              &maxW,
		Type:                 &dtype,
	}

	data, err := xml.Marshal(&derCap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	xmlStr := string(data)

	assertOrder(t, xmlStr, []string{
		"<modesSupported>",
		"<rtgMaxA>",
		"<rtgMaxChargeRateW>",
		"<rtgMaxDischargeRateW>",
		"<rtgMaxVar>",
		"<rtgMaxW>",
		"<type>",
	})
}

// TestDERSettingsWireOrder asserts DERSettings order against sep.xsd's
// sequence: modesEnabled, setESDelay, ..., setMaxChargeRateW,
// setMaxDischargeRateW, ..., setMaxVar, setMaxVarNeg, setMaxW, ...,
// updatedTime (last). The regression this guards: setMaxVar/setMaxW were
// previously emitted before setMaxChargeRateW/setMaxDischargeRateW.
func TestDERSettingsWireOrder(t *testing.T) {
	modes := sep2.DERControlType(0x0F)
	maxW := sep2.ActivePower{Value: 5000}
	maxVar := sep2.ReactivePower{Value: 100}
	maxChargeW := sep2.ActivePower{Value: 200}
	maxDischargeW := sep2.ActivePower{Value: 300}

	settings := sep2.DERSettings{
		ModesEnabled:         &modes,
		SetMaxW:              &maxW,
		SetMaxVar:            &maxVar,
		SetMaxChargeRateW:    &maxChargeW,
		SetMaxDischargeRateW: &maxDischargeW,
		UpdatedTime:          1604963587,
	}

	data, err := xml.Marshal(&settings)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	xmlStr := string(data)

	assertOrder(t, xmlStr, []string{
		"<modesEnabled>",
		"<setMaxChargeRateW>",
		"<setMaxDischargeRateW>",
		"<setMaxVar>",
		"<setMaxW>",
		"<updatedTime>",
	})
}

// TestDERStatusWireOrder asserts DERStatus order against sep.xsd's sequence:
// alarmStatus, genConnectStatus, inverterStatus, localControlModeStatus,
// manufacturerStatus, operationalModeStatus, readingTime,
// stateOfChargeStatus, storageModeStatus, storConnectStatus. The
// regression this guards: alarmStatus was previously emitted near-last,
// after readingTime, instead of first.
func TestDERStatusWireOrder(t *testing.T) {
	alarm := sep2.HexBinary32(0x01)
	status := sep2.DERStatus{
		AlarmStatus:           &alarm,
		GenConnectStatus:      &sep2.ConnectStatusType{Value: 1},
		InverterStatus:        &sep2.InverterStatusType{Value: 2},
		OperationalModeStatus: &sep2.OperationalModeStatusType{Value: 3},
		ReadingTime:           1604963587,
		StateOfChargeStatus:   &sep2.StateOfChargeStatusType{DateTime: 1604963587, Value: 8500},
		StorageModeStatus:     &sep2.StorageModeStatusType{DateTime: 1604963587, Value: 1},
	}

	data, err := xml.Marshal(&status)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	xmlStr := string(data)

	assertOrder(t, xmlStr, []string{
		"<alarmStatus>",
		"<genConnectStatus>",
		"<inverterStatus>",
		"<operationalModeStatus>",
		"<readingTime>",
		"<stateOfChargeStatus>",
		"<storageModeStatus>",
	})
}

// TestDERProgramWireOrder asserts DERProgram order against sep.xsd's
// sequence: ActiveDERControlListLink, DefaultDERControlLink,
// DERControlListLink, DERCurveListLink, primacy (LAST). The regression
// this guards: primacy was previously emitted third, ahead of the DER*
// link fields, instead of last.
func TestDERProgramWireOrder(t *testing.T) {
	prog := sep2.DERProgram{
		SubscribableResource: sep2.SubscribableResource{
			Resource: sep2.Resource{Href: "/derp/1"},
		},
		MRID:                     "PROG001",
		Description:              "Solar DER Program",
		Primacy:                  10,
		ActiveDERControlListLink: &sep2.ListLink{Href: "/derp/1/actderc"},
		DefaultDERControlLink:    &sep2.Link{Href: "/derp/1/dderc"},
		DERControlListLink:       &sep2.ListLink{Href: "/derp/1/derc"},
		DERCurveListLink:         &sep2.ListLink{Href: "/derp/1/dc"},
	}

	data, err := xml.Marshal(&prog)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	xmlStr := string(data)

	assertOrder(t, xmlStr, []string{
		"<ActiveDERControlListLink",
		"<DefaultDERControlLink",
		"<DERControlListLink",
		"<DERCurveListLink",
		"<primacy>",
	})
}

// TestDERControlBaseWireOrder asserts DERControlBase order against
// sep.xsd's sequence for the two field pairs Pike flagged as swapped:
// opModFixedVar before opModFixedW, and opModTargetVar before opModTargetW.
func TestDERControlBaseWireOrder(t *testing.T) {
	fixedW := sep2.SignedPerCent(100)
	fixedVar := sep2.ReactivePower{Value: 200}
	targetW := sep2.ActivePower{Value: 300}
	targetVar := sep2.ReactivePower{Value: 400}

	base := sep2.DERControlBase{
		OpModFixedW:    &fixedW,
		OpModFixedVar:  &fixedVar,
		OpModTargetW:   &targetW,
		OpModTargetVar: &targetVar,
	}
	ctrl := sep2.DERControl{DERControlBase: &base}

	data, err := xml.Marshal(&ctrl)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	xmlStr := string(data)

	assertOrder(t, xmlStr, []string{
		"<opModFixedVar>",
		"<opModFixedW>",
	})
	assertOrder(t, xmlStr, []string{
		"<opModTargetVar>",
		"<opModTargetW>",
	})
}
