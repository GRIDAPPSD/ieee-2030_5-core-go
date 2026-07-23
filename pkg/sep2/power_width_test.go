package sep2_test

import (
	"encoding/xml"
	"strconv"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// TestActivePowerValueBoundaryInt16 asserts ActivePower.Value marshals
// correctly at the xs:short (XSD Int16) boundary values -32768 and 32767.
// Value was previously Go int64, which permits wire values outside the
// XSD Int16 range; a strict client validating against sep.xsd's
// ActivePower/Int16 base type would reject an out-of-range value. The
// Go field type itself (int16) is now the enforcement mechanism: an
// out-of-range assignment is a compile error, not a runtime wire defect.
func TestActivePowerValueBoundaryInt16(t *testing.T) {
	cases := []struct {
		name string
		val  int16
	}{
		{"max", 32767},
		{"min", -32768},
		{"zero", 0},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := sep2.ActivePower{Multiplier: 0, Value: tc.val}
			data, err := xml.Marshal(&p)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			xmlStr := string(data)
			want := "<value>" + strconv.FormatInt(int64(tc.val), 10) + "</value>"
			if !strings.Contains(xmlStr, want) {
				t.Errorf("missing %q in XML: %s", want, xmlStr)
			}

			var parsed sep2.ActivePower
			if err := xml.Unmarshal(data, &parsed); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if parsed.Value != tc.val {
				t.Errorf("round-trip Value = %d, want %d", parsed.Value, tc.val)
			}
		})
	}
}

// TestReactivePowerValueBoundaryInt16 mirrors
// TestActivePowerValueBoundaryInt16 for ReactivePower.
func TestReactivePowerValueBoundaryInt16(t *testing.T) {
	cases := []struct {
		name string
		val  int16
	}{
		{"max", 32767},
		{"min", -32768},
		{"zero", 0},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := sep2.ReactivePower{Multiplier: 0, Value: tc.val}
			data, err := xml.Marshal(&p)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			xmlStr := string(data)
			want := "<value>" + strconv.FormatInt(int64(tc.val), 10) + "</value>"
			if !strings.Contains(xmlStr, want) {
				t.Errorf("missing %q in XML: %s", want, xmlStr)
			}

			var parsed sep2.ReactivePower
			if err := xml.Unmarshal(data, &parsed); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if parsed.Value != tc.val {
				t.Errorf("round-trip Value = %d, want %d", parsed.Value, tc.val)
			}
		})
	}
}
