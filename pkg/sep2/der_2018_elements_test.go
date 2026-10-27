package sep2_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

const rtgMaxACompact = `<DERCapability xmlns="urn:ieee:std:2030.5:ns"><rtgMaxA><multiplier>-1</multiplier><value>205</value></rtgMaxA></DERCapability>`

const rtgMaxAIndented = `<DERCapability xmlns="urn:ieee:std:2030.5:ns">
  <rtgMaxA>
    <multiplier>-1</multiplier>
    <value>205</value>
  </rtgMaxA>
</DERCapability>`

// Compact and indented XML must both decode rtgMaxA as multiplier and value.
// With the old *int32 type, compact XML decoded to 0 and indented XML failed.
func TestDERCapabilityRTGMaxADecodesMultiplierAndValue(t *testing.T) {
	for name, doc := range map[string]string{"compact": rtgMaxACompact, "indented": rtgMaxAIndented} {
		t.Run(name, func(t *testing.T) {
			var c sep2.DERCapability
			if err := xml.Unmarshal([]byte(doc), &c); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			out, err := xml.Marshal(c)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			want := "<rtgMaxA><multiplier>-1</multiplier><value>205</value></rtgMaxA>"
			if !strings.Contains(string(out), want) {
				t.Errorf("re-marshalled = %s, want it to contain %s", out, want)
			}
		})
	}
}

func TestDERCapability2018ElementsRoundTrip(t *testing.T) {
	u8 := func(v uint8) *uint8 { return &v }
	ap := func(v uint16) *sep2.ApparentPower { return &sep2.ApparentPower{Multiplier: 1, Value: v} }
	act := func(v int16) *sep2.ActivePower { return &sep2.ActivePower{Multiplier: 2, Value: v} }
	pf := func(v uint16) *sep2.PowerFactor { return &sep2.PowerFactor{Displacement: v, Multiplier: -3} }
	in := sep2.DERCapability{
		RTGAbnormalCategory:   u8(2),
		RTGMaxA:               &sep2.CurrentRMS{Multiplier: -1, Value: 205},
		RTGMaxAh:              &sep2.AmpereHour{Multiplier: 0, Value: 300},
		RTGMaxChargeRateVA:    ap(11),
		RTGMaxDischargeRateVA: ap(12),
		RTGMaxVarNeg:          &sep2.ReactivePower{Multiplier: 3, Value: -13},
		RTGMaxWh:              &sep2.WattHour{Multiplier: 3, Value: 14},
		RTGMinPFOverExcited:   pf(950),
		RTGMinPFUnderExcited:  pf(900),
		RTGMinV:               &sep2.VoltageRMS{Multiplier: -1, Value: 1900},
		RTGNormalCategory:     u8(3),
		RTGOverExcitedPF:      pf(850),
		RTGOverExcitedW:       act(15),
		RTGReactiveSusceptance: &sep2.ReactiveSusceptance{
			Multiplier: -2, Value: 16},
		RTGUnderExcitedPF: pf(800),
		RTGUnderExcitedW:  act(17),
		RTGVNom:           &sep2.VoltageRMS{Multiplier: -1, Value: 2400},
	}
	data, err := xml.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	assertOrder(t, string(data), []string{
		"<rtgAbnormalCategory>", "<rtgMaxA>", "<rtgMaxAh>", "<rtgMaxChargeRateVA>",
		"<rtgMaxDischargeRateVA>", "<rtgMaxVarNeg>", "<rtgMaxWh>",
		"<rtgMinPFOverExcited>", "<rtgMinPFUnderExcited>", "<rtgMinV>",
		"<rtgNormalCategory>", "<rtgOverExcitedPF>", "<rtgOverExcitedW>",
		"<rtgReactiveSusceptance>", "<rtgUnderExcitedPF>", "<rtgUnderExcitedW>",
		"<rtgVNom>",
	})
	var out sep2.DERCapability
	if err := xml.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	checks := []struct {
		name      string
		got, want any
	}{
		{"RTGAbnormalCategory", *out.RTGAbnormalCategory, uint8(2)},
		{"RTGMaxA", *out.RTGMaxA, *in.RTGMaxA},
		{"RTGMaxAh", *out.RTGMaxAh, *in.RTGMaxAh},
		{"RTGMaxChargeRateVA", *out.RTGMaxChargeRateVA, *in.RTGMaxChargeRateVA},
		{"RTGMaxDischargeRateVA", *out.RTGMaxDischargeRateVA, *in.RTGMaxDischargeRateVA},
		{"RTGMaxVarNeg", *out.RTGMaxVarNeg, *in.RTGMaxVarNeg},
		{"RTGMaxWh", *out.RTGMaxWh, *in.RTGMaxWh},
		{"RTGMinPFOverExcited", *out.RTGMinPFOverExcited, *in.RTGMinPFOverExcited},
		{"RTGMinPFUnderExcited", *out.RTGMinPFUnderExcited, *in.RTGMinPFUnderExcited},
		{"RTGMinV", *out.RTGMinV, *in.RTGMinV},
		{"RTGNormalCategory", *out.RTGNormalCategory, uint8(3)},
		{"RTGOverExcitedPF", *out.RTGOverExcitedPF, *in.RTGOverExcitedPF},
		{"RTGOverExcitedW", *out.RTGOverExcitedW, *in.RTGOverExcitedW},
		{"RTGReactiveSusceptance", *out.RTGReactiveSusceptance, *in.RTGReactiveSusceptance},
		{"RTGUnderExcitedPF", *out.RTGUnderExcitedPF, *in.RTGUnderExcitedPF},
		{"RTGUnderExcitedW", *out.RTGUnderExcitedW, *in.RTGUnderExcitedW},
		{"RTGVNom", *out.RTGVNom, *in.RTGVNom},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %+v, want %+v", c.name, c.got, c.want)
		}
	}
}

// Copy must not alias any pointer field, including the new ones.
func TestDERCapabilityCopyIndependentNewFields(t *testing.T) {
	in := sep2.DERCapability{
		RTGMaxA:  &sep2.CurrentRMS{Value: 1},
		RTGMaxWh: &sep2.WattHour{Value: 2},
		RTGVNom:  &sep2.VoltageRMS{Value: 3},
	}
	c := in.Copy()
	c.RTGMaxA.Value, c.RTGMaxWh.Value, c.RTGVNom.Value = 9, 9, 9
	if in.RTGMaxA.Value != 1 || in.RTGMaxWh.Value != 2 || in.RTGVNom.Value != 3 {
		t.Errorf("Copy aliased: %+v %+v %+v", *in.RTGMaxA, *in.RTGMaxWh, *in.RTGVNom)
	}
}

func TestDERStatusLocalControlAndManufacturerRoundTrip(t *testing.T) {
	in := sep2.DERStatus{
		InverterStatus:         &sep2.InverterStatusType{DateTime: 100, Value: 4},
		LocalControlModeStatus: &sep2.LocalControlModeStatusType{DateTime: 101, Value: 1},
		ManufacturerStatus:     &sep2.ManufacturerStatusType{DateTime: 102, Value: "OK1234"},
		ReadingTime:            103,
	}
	data, err := xml.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	assertOrder(t, string(data), []string{
		"<inverterStatus>", "<localControlModeStatus>", "<manufacturerStatus>", "<readingTime>",
	})
	var out sep2.DERStatus
	if err := xml.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if *out.LocalControlModeStatus != *in.LocalControlModeStatus {
		t.Errorf("LocalControlModeStatus = %+v, want %+v", *out.LocalControlModeStatus, *in.LocalControlModeStatus)
	}
	if *out.ManufacturerStatus != *in.ManufacturerStatus {
		t.Errorf("ManufacturerStatus = %+v, want %+v", *out.ManufacturerStatus, *in.ManufacturerStatus)
	}
	c := in.Copy()
	c.LocalControlModeStatus.Value, c.ManufacturerStatus.Value = 0, "X"
	if in.LocalControlModeStatus.Value != 1 || in.ManufacturerStatus.Value != "OK1234" {
		t.Error("DERStatus.Copy aliased the new fields")
	}
}
