package sep2_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// TestDERCurveTypeConstants pins every CurveTypeOpMod... constant to the
// DERCurveType (UInt8) table in IEEE 2030.5-2018.
func TestDERCurveTypeConstants(t *testing.T) {
	tests := []struct {
		name string
		got  uint8
		want uint8
	}{
		{"CurveTypeOpModFreqWatt", sep2.CurveTypeOpModFreqWatt, 0},
		{"CurveTypeOpModHFRTMayTrip", sep2.CurveTypeOpModHFRTMayTrip, 1},
		{"CurveTypeOpModHFRTMustTrip", sep2.CurveTypeOpModHFRTMustTrip, 2},
		{"CurveTypeOpModHVRTMayTrip", sep2.CurveTypeOpModHVRTMayTrip, 3},
		{"CurveTypeOpModHVRTMomentaryCessation", sep2.CurveTypeOpModHVRTMomentaryCessation, 4},
		{"CurveTypeOpModHVRTMustTrip", sep2.CurveTypeOpModHVRTMustTrip, 5},
		{"CurveTypeOpModLFRTMayTrip", sep2.CurveTypeOpModLFRTMayTrip, 6},
		{"CurveTypeOpModLFRTMustTrip", sep2.CurveTypeOpModLFRTMustTrip, 7},
		{"CurveTypeOpModLVRTMayTrip", sep2.CurveTypeOpModLVRTMayTrip, 8},
		{"CurveTypeOpModLVRTMomentaryCessation", sep2.CurveTypeOpModLVRTMomentaryCessation, 9},
		{"CurveTypeOpModLVRTMustTrip", sep2.CurveTypeOpModLVRTMustTrip, 10},
		{"CurveTypeOpModVoltVar", sep2.CurveTypeOpModVoltVar, 11},
		{"CurveTypeOpModVoltWatt", sep2.CurveTypeOpModVoltWatt, 12},
		{"CurveTypeOpModWattPF", sep2.CurveTypeOpModWattPF, 13},
		{"CurveTypeOpModWattVar", sep2.CurveTypeOpModWattVar, 14},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s = %d, want %d", tc.name, tc.got, tc.want)
			}
		})
	}
}

// TestDERCurveTypeUndefinedValuePassesThrough covers 15-255, which sep.xsd
// reserves and does not name. DERCurve.CurveType has no custom decode
// validation, so an out-of-table value round-trips unchanged rather than
// being rejected.
func TestDERCurveTypeUndefinedValuePassesThrough(t *testing.T) {
	const doc = `<DERCurve xmlns="urn:ieee:std:2030.5:ns">` +
		`<mRID>0102030405060708090A0B0C0D0E0F10</mRID>` +
		`<creationTime>0</creationTime>` +
		`<curveType>255</curveType>` +
		`<xMultiplier>0</xMultiplier>` +
		`<yMultiplier>0</yMultiplier>` +
		`<yRefType>0</yRefType>` +
		`</DERCurve>`

	var c sep2.DERCurve
	if err := xml.Unmarshal([]byte(doc), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.CurveType != 255 {
		t.Errorf("CurveType = %d, want 255 (reserved value passed through)", c.CurveType)
	}

	data, err := xml.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), "<curveType>255</curveType>") {
		t.Errorf("re-encoded DERCurve lost reserved curveType 255\nXML: %s", data)
	}
}
