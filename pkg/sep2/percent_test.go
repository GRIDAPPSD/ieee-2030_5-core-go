package sep2_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// TestPerCentRoundTrip asserts PerCent marshals as bare element text (not
// multiplier+value children) and round-trips across IEEE 2030.5-2018's
// UInt16 hundredths-of-a-percent range, 0 to 10000.
func TestPerCentRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		val  sep2.PerCent
		want string
	}{
		{"zero", 0, "0"},
		{"half", 5000, "5000"},
		{"max", 10000, "10000"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data, err := xml.Marshal(tc.val)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			xmlStr := string(data)
			if !strings.Contains(xmlStr, ">"+tc.want+"<") {
				t.Errorf("xml = %s, want value text %s", xmlStr, tc.want)
			}
			if strings.Contains(xmlStr, "<value>") || strings.Contains(xmlStr, "<multiplier>") {
				t.Errorf("xml = %s, PerCent is a schema simple type: no multiplier/value children", xmlStr)
			}

			var parsed sep2.PerCent
			if err := xml.Unmarshal(data, &parsed); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if parsed != tc.val {
				t.Errorf("round-trip = %d, want %d", parsed, tc.val)
			}
		})
	}
}

// TestSignedPerCentRoundTrip mirrors TestPerCentRoundTrip for SignedPerCent's
// Int16 range, -10000 to 10000, including a negative value.
func TestSignedPerCentRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		val  sep2.SignedPerCent
		want string
	}{
		{"zero", 0, "0"},
		{"positive", 5000, "5000"},
		{"max", 10000, "10000"},
		{"negative", -2500, "-2500"},
		{"min", -10000, "-10000"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data, err := xml.Marshal(tc.val)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			xmlStr := string(data)
			if !strings.Contains(xmlStr, ">"+tc.want+"<") {
				t.Errorf("xml = %s, want value text %s", xmlStr, tc.want)
			}

			var parsed sep2.SignedPerCent
			if err := xml.Unmarshal(data, &parsed); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if parsed != tc.val {
				t.Errorf("round-trip = %d, want %d", parsed, tc.val)
			}
		})
	}
}

// TestOpModFixedWWireIsScalarPercent asserts DERControlBase.OpModFixedW
// marshals as bare opModFixedW element text, matching the SignedPerCent
// schema simple type (IEEE 2030.5-2018 Annex B.2.22), not the
// multiplier+value ActivePower complex type it previously carried.
func TestOpModFixedWWireIsScalarPercent(t *testing.T) {
	v := sep2.SignedPerCent(-2500)
	base := sep2.DERControlBase{OpModFixedW: &v}

	data, err := xml.Marshal(&base)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	xmlStr := string(data)
	if !strings.Contains(xmlStr, "<opModFixedW>-2500</opModFixedW>") {
		t.Errorf("xml = %s, want <opModFixedW>-2500</opModFixedW>", xmlStr)
	}

	var parsed sep2.DERControlBase
	if err := xml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.OpModFixedW == nil || *parsed.OpModFixedW != -2500 {
		t.Errorf("round-trip OpModFixedW = %v, want -2500", parsed.OpModFixedW)
	}
}

// TestOpModMaxLimWWireIsScalarPercent mirrors
// TestOpModFixedWWireIsScalarPercent for OpModMaxLimW (PerCent).
func TestOpModMaxLimWWireIsScalarPercent(t *testing.T) {
	v := sep2.PerCent(7500)
	base := sep2.DERControlBase{OpModMaxLimW: &v}

	data, err := xml.Marshal(&base)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	xmlStr := string(data)
	if !strings.Contains(xmlStr, "<opModMaxLimW>7500</opModMaxLimW>") {
		t.Errorf("xml = %s, want <opModMaxLimW>7500</opModMaxLimW>", xmlStr)
	}

	var parsed sep2.DERControlBase
	if err := xml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.OpModMaxLimW == nil || *parsed.OpModMaxLimW != 7500 {
		t.Errorf("round-trip OpModMaxLimW = %v, want 7500", parsed.OpModMaxLimW)
	}
}
