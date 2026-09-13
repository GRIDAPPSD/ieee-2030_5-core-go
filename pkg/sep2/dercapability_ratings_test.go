package sep2_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// derCapabilityDoc carries rtgMaxVA at 65000, a value only an unsigned 16-bit
// field holds, because sep.xsd types ApparentPower and VoltageRMS values UInt16.
const derCapabilityDoc = `<DERCapability xmlns="urn:ieee:std:2030.5:ns" href="/edev/1/der/1/dercap">` +
	`<modesSupported>0800</modesSupported>` +
	`<rtgMaxDischargeRateW><multiplier>0</multiplier><value>300</value></rtgMaxDischargeRateW>` +
	`<rtgMaxV><multiplier>-1</multiplier><value>2400</value></rtgMaxV>` +
	`<rtgMaxVA><multiplier>0</multiplier><value>65000</value></rtgMaxVA>` +
	`<rtgMaxVar><multiplier>0</multiplier><value>100</value></rtgMaxVar>` +
	`<rtgMaxW><multiplier>3</multiplier><value>9</value></rtgMaxW>` +
	`<type>4</type>` +
	`</DERCapability>`

func decodeDERCapabilityDoc(t *testing.T) sep2.DERCapability {
	t.Helper()
	var c sep2.DERCapability
	if err := xml.Unmarshal([]byte(derCapabilityDoc), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return c
}

func TestDERCapabilityDecodesRatings(t *testing.T) {
	c := decodeDERCapabilityDoc(t)
	if c.RTGMaxV == nil || *c.RTGMaxV != (sep2.VoltageRMS{Multiplier: -1, Value: 2400}) {
		t.Errorf("RTGMaxV = %+v, want {Multiplier:-1 Value:2400}", c.RTGMaxV)
	}
	if c.RTGMaxVA == nil || *c.RTGMaxVA != (sep2.ApparentPower{Multiplier: 0, Value: 65000}) {
		t.Errorf("RTGMaxVA = %+v, want {Multiplier:0 Value:65000}", c.RTGMaxVA)
	}
}

func TestDERCapabilityRatingsSurviveDecodeEncode(t *testing.T) {
	data, err := xml.Marshal(decodeDERCapabilityDoc(t))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(data)
	for _, want := range []string{
		"<rtgMaxV><multiplier>-1</multiplier><value>2400</value></rtgMaxV>",
		"<rtgMaxVA><multiplier>0</multiplier><value>65000</value></rtgMaxVA>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("re-encoded DERCapability lacks %s\nXML: %s", want, got)
		}
	}
}

// TestDERCapabilityRatingsWireOrder populates every modelled element, so
// rtgMaxV and rtgMaxVA are checked against both neighbours in the sep.xsd
// sequence.
func TestDERCapabilityRatingsWireOrder(t *testing.T) {
	modes := sep2.DERControlType(0xFF)
	maxA := int32(20)
	derCap := sep2.DERCapability{
		ModesSupported:       &modes,
		RTGMaxA:              &maxA,
		RTGMaxChargeRateW:    &sep2.ActivePower{Value: 200},
		RTGMaxDischargeRateW: &sep2.ActivePower{Value: 300},
		RTGMaxV:              &sep2.VoltageRMS{Value: 240},
		RTGMaxVA:             &sep2.ApparentPower{Value: 6000},
		RTGMaxVar:            &sep2.ReactivePower{Value: 100},
		RTGMaxW:              &sep2.ActivePower{Value: 5000},
		Type:                 func() *uint8 { v := uint8(4); return &v }(),
	}

	data, err := xml.Marshal(&derCap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	assertOrder(t, string(data), []string{
		"<modesSupported>",
		"<rtgMaxA>",
		"<rtgMaxChargeRateW>",
		"<rtgMaxDischargeRateW>",
		"<rtgMaxV>",
		"<rtgMaxVA>",
		"<rtgMaxVar>",
		"<rtgMaxW>",
		"<type>",
	})
}

// Both ratings are minOccurs=0, so an unset one is absent, never an empty
// element.
func TestDERCapabilityZeroValueOmitsRatings(t *testing.T) {
	data, err := xml.Marshal(sep2.DERCapability{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, name := range []string{"rtgMaxV>", "rtgMaxVA>"} {
		if strings.Contains(string(data), name) {
			t.Errorf("zero-value DERCapability emitted %s\nXML: %s", name, data)
		}
	}
}

func TestDERCapabilityCopyRatingsIndependent(t *testing.T) {
	original := decodeDERCapabilityDoc(t)
	copied := original.Copy()

	if copied.RTGMaxV == original.RTGMaxV || copied.RTGMaxVA == original.RTGMaxVA {
		t.Fatal("Copy shares a rating pointer with the original")
	}
	if *copied.RTGMaxV != *original.RTGMaxV || *copied.RTGMaxVA != *original.RTGMaxVA {
		t.Errorf("Copy changed a rating: got V=%+v VA=%+v", *copied.RTGMaxV, *copied.RTGMaxVA)
	}

	copied.RTGMaxV.Value = 1
	copied.RTGMaxVA.Value = 1
	if original.RTGMaxV.Value != 2400 || original.RTGMaxVA.Value != 65000 {
		t.Errorf("mutating the copy changed the original: V=%+v VA=%+v", *original.RTGMaxV, *original.RTGMaxVA)
	}
}
