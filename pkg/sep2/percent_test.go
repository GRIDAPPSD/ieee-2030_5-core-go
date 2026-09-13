package sep2_test

import (
	"encoding/json"
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
			if strings.Contains(xmlStr, "<value>") || strings.Contains(xmlStr, "<multiplier>") {
				t.Errorf("xml = %s, SignedPerCent is a schema simple type: no multiplier/value children", xmlStr)
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

// TestPerCentDecodeRejectsMalformed pins the wire-boundary refusals a bare
// default xml.Unmarshal does not give a named integer type: the pre-fix
// multiplier+value shape, empty text, non-integer text, and out-of-range
// values must all error rather than decode to a silent zero or an
// out-of-spec value.
func TestPerCentDecodeRejectsMalformed(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr bool
		wantVal sep2.PerCent
	}{
		{"valid zero", "<PerCent>0</PerCent>", false, 0},
		{"valid max", "<PerCent>10000</PerCent>", false, 10000},
		{"valid with surrounding whitespace", "<PerCent> 5000 </PerCent>", false, 5000},
		{"old compact multiplier+value shape", "<PerCent><multiplier>0</multiplier><value>5000</value></PerCent>", true, 0},
		{"old indented multiplier+value shape", "<PerCent>\n  <multiplier>0</multiplier>\n  <value>5000</value>\n</PerCent>", true, 0},
		{"self-closed empty element", "<PerCent/>", true, 0},
		{"empty element", "<PerCent></PerCent>", true, 0},
		{"whitespace-only element", "<PerCent>   </PerCent>", true, 0},
		{"non-integer", "<PerCent>12.5</PerCent>", true, 0},
		{"negative", "<PerCent>-1</PerCent>", true, 0},
		{"out of spec range", "<PerCent>10001</PerCent>", true, 0},
		{"in uint16 range but out of spec range", "<PerCent>65535</PerCent>", true, 0},
		{"beyond uint16", "<PerCent>65536</PerCent>", true, 0},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got sep2.PerCent
			err := xml.Unmarshal([]byte(tc.body), &got)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Unmarshal(%s) = %d, nil, want an error", tc.body, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Unmarshal(%s): %v", tc.body, err)
			}
			if got != tc.wantVal {
				t.Errorf("Unmarshal(%s) = %d, want %d", tc.body, got, tc.wantVal)
			}
		})
	}
}

// TestSignedPerCentDecodeRejectsMalformed mirrors
// TestPerCentDecodeRejectsMalformed for the signed range.
func TestSignedPerCentDecodeRejectsMalformed(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr bool
		wantVal sep2.SignedPerCent
	}{
		{"valid zero", "<SignedPerCent>0</SignedPerCent>", false, 0},
		{"valid max", "<SignedPerCent>10000</SignedPerCent>", false, 10000},
		{"valid min", "<SignedPerCent>-10000</SignedPerCent>", false, -10000},
		{"old compact multiplier+value shape", "<SignedPerCent><multiplier>0</multiplier><value>5000</value></SignedPerCent>", true, 0},
		{"self-closed empty element", "<SignedPerCent/>", true, 0},
		{"empty element", "<SignedPerCent></SignedPerCent>", true, 0},
		{"whitespace-only element", "<SignedPerCent>   </SignedPerCent>", true, 0},
		{"non-integer", "<SignedPerCent>1e3</SignedPerCent>", true, 0},
		{"out of spec range positive", "<SignedPerCent>10001</SignedPerCent>", true, 0},
		{"out of spec range negative", "<SignedPerCent>-10001</SignedPerCent>", true, 0},
		{"in int16 range but out of spec range", "<SignedPerCent>32767</SignedPerCent>", true, 0},
		{"beyond int16", "<SignedPerCent>32768</SignedPerCent>", true, 0},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got sep2.SignedPerCent
			err := xml.Unmarshal([]byte(tc.body), &got)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Unmarshal(%s) = %d, nil, want an error", tc.body, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Unmarshal(%s): %v", tc.body, err)
			}
			if got != tc.wantVal {
				t.Errorf("Unmarshal(%s) = %d, want %d", tc.body, got, tc.wantVal)
			}
		})
	}
}

// TestDERControlBaseOldWireShapeRejected proves the mixed-version hazard
// (a peer still emitting the pre-fix ActivePower multiplier+value shape)
// errors instead of silently landing a zero setpoint in DERControlBase.
func TestDERControlBaseOldWireShapeRejected(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			"OpModMaxLimW old shape",
			`<DERControlBase><opModMaxLimW><multiplier>0</multiplier><value>5000</value></opModMaxLimW></DERControlBase>`,
		},
		{
			"OpModFixedW old shape",
			`<DERControlBase><opModFixedW><multiplier>0</multiplier><value>5000</value></opModFixedW></DERControlBase>`,
		},
		{
			"OpModMaxLimW empty element",
			`<DERControlBase><opModMaxLimW></opModMaxLimW></DERControlBase>`,
		},
		{
			"OpModFixedW empty element",
			`<DERControlBase><opModFixedW></opModFixedW></DERControlBase>`,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got sep2.DERControlBase
			err := xml.Unmarshal([]byte(tc.body), &got)
			if err == nil {
				t.Fatalf("Unmarshal(%s) = %+v, nil, want an error", tc.body, got)
			}
		})
	}
}

// TestPerCentEncodeRejectsOutOfRange proves MarshalXML refuses to serialize
// an out-of-spec value, matching RealEnergy's boundary guard.
func TestPerCentEncodeRejectsOutOfRange(t *testing.T) {
	if _, err := xml.Marshal(sep2.PerCent(10000)); err != nil {
		t.Fatalf("Marshal(10000): %v, want nil", err)
	}
	if _, err := xml.Marshal(sep2.PerCent(10001)); err == nil {
		t.Fatal("Marshal(10001) = nil error, want an error")
	}
	if _, err := xml.Marshal(sep2.PerCent(65535)); err == nil {
		t.Fatal("Marshal(65535) = nil error, want an error")
	}
}

// TestSignedPerCentEncodeRejectsOutOfRange mirrors
// TestPerCentEncodeRejectsOutOfRange for the signed range.
func TestSignedPerCentEncodeRejectsOutOfRange(t *testing.T) {
	if _, err := xml.Marshal(sep2.SignedPerCent(-10000)); err != nil {
		t.Fatalf("Marshal(-10000): %v, want nil", err)
	}
	if _, err := xml.Marshal(sep2.SignedPerCent(10001)); err == nil {
		t.Fatal("Marshal(10001) = nil error, want an error")
	}
	if _, err := xml.Marshal(sep2.SignedPerCent(-32768)); err == nil {
		t.Fatal("Marshal(-32768) = nil error, want an error")
	}
}

// TestPerCentJSON proves JSON decode and encode enforce the same range as
// XML; server-go ingests JSON fixtures for this package's types.
func TestPerCentJSON(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr bool
		wantVal sep2.PerCent
	}{
		{"valid zero", "0", false, 0},
		{"valid max", "10000", false, 10000},
		{"out of spec range", "10001", true, 0},
		{"negative", "-1", true, 0},
		{"beyond uint16", "70000", true, 0},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got sep2.PerCent
			err := json.Unmarshal([]byte(tc.body), &got)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("json.Unmarshal(%s) = %d, nil, want an error", tc.body, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("json.Unmarshal(%s): %v", tc.body, err)
			}
			if got != tc.wantVal {
				t.Errorf("json.Unmarshal(%s) = %d, want %d", tc.body, got, tc.wantVal)
			}
		})
	}

	if _, err := json.Marshal(sep2.PerCent(10001)); err == nil {
		t.Fatal("json.Marshal(10001) = nil error, want an error")
	}
	data, err := json.Marshal(sep2.PerCent(7500))
	if err != nil {
		t.Fatalf("json.Marshal(7500): %v", err)
	}
	if string(data) != "7500" {
		t.Errorf("json.Marshal(7500) = %s, want bare number 7500", data)
	}
}

// TestSignedPerCentJSON mirrors TestPerCentJSON for the signed range.
func TestSignedPerCentJSON(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr bool
		wantVal sep2.SignedPerCent
	}{
		{"valid min", "-10000", false, -10000},
		{"valid max", "10000", false, 10000},
		{"out of spec range positive", "10001", true, 0},
		{"out of spec range negative", "-10001", true, 0},
		{"beyond int16", "32768", true, 0},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got sep2.SignedPerCent
			err := json.Unmarshal([]byte(tc.body), &got)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("json.Unmarshal(%s) = %d, nil, want an error", tc.body, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("json.Unmarshal(%s): %v", tc.body, err)
			}
			if got != tc.wantVal {
				t.Errorf("json.Unmarshal(%s) = %d, want %d", tc.body, got, tc.wantVal)
			}
		})
	}

	if _, err := json.Marshal(sep2.SignedPerCent(-10001)); err == nil {
		t.Fatal("json.Marshal(-10001) = nil error, want an error")
	}
}

// TestOpModFixedWWireIsScalarPercent asserts DERControlBase.OpModFixedW
// marshals as bare opModFixedW element text, matching the SignedPerCent
// schema simple type (IEEE 2030.5-2018 Annex B.2.22), not the
// multiplier+value ActivePower complex type it previously carried, and
// round-trips zero and both range boundaries.
func TestOpModFixedWWireIsScalarPercent(t *testing.T) {
	cases := []struct {
		name string
		val  sep2.SignedPerCent
		want string
	}{
		{"negative", -2500, "-2500"},
		{"zero", 0, "0"},
		{"max", 10000, "10000"},
		{"min", -10000, "-10000"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v := tc.val
			base := sep2.DERControlBase{OpModFixedW: &v}

			data, err := xml.Marshal(&base)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			xmlStr := string(data)
			want := "<opModFixedW>" + tc.want + "</opModFixedW>"
			if !strings.Contains(xmlStr, want) {
				t.Errorf("xml = %s, want %s", xmlStr, want)
			}

			var parsed sep2.DERControlBase
			if err := xml.Unmarshal(data, &parsed); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if parsed.OpModFixedW == nil {
				t.Fatalf("round-trip OpModFixedW = nil, want %d", tc.val)
			}
			if *parsed.OpModFixedW != tc.val {
				t.Errorf("round-trip OpModFixedW = %d, want %d", *parsed.OpModFixedW, tc.val)
			}
		})
	}
}

// TestOpModMaxLimWWireIsScalarPercent mirrors
// TestOpModFixedWWireIsScalarPercent for OpModMaxLimW (PerCent).
func TestOpModMaxLimWWireIsScalarPercent(t *testing.T) {
	cases := []struct {
		name string
		val  sep2.PerCent
		want string
	}{
		{"mid", 7500, "7500"},
		{"zero", 0, "0"},
		{"max", 10000, "10000"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v := tc.val
			base := sep2.DERControlBase{OpModMaxLimW: &v}

			data, err := xml.Marshal(&base)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			xmlStr := string(data)
			want := "<opModMaxLimW>" + tc.want + "</opModMaxLimW>"
			if !strings.Contains(xmlStr, want) {
				t.Errorf("xml = %s, want %s", xmlStr, want)
			}

			var parsed sep2.DERControlBase
			if err := xml.Unmarshal(data, &parsed); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if parsed.OpModMaxLimW == nil {
				t.Fatalf("round-trip OpModMaxLimW = nil, want %d", tc.val)
			}
			if *parsed.OpModMaxLimW != tc.val {
				t.Errorf("round-trip OpModMaxLimW = %d, want %d", *parsed.OpModMaxLimW, tc.val)
			}
		})
	}
}
