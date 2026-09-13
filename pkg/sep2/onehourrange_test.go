package sep2_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// TestOneHourRangeRoundTrip mirrors TestSignedPerCentRoundTrip for
// OneHourRangeType (Int16, restricted to -3600..3600 by sep.xsd), the
// randomizeStart/randomizeDuration base per #111.
func TestOneHourRangeRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		val  sep2.OneHourRange
		want string
	}{
		{"zero", 0, "0"},
		{"max", 3600, "3600"},
		{"min", -3600, "-3600"},
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
			var parsed sep2.OneHourRange
			if err := xml.Unmarshal(data, &parsed); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if parsed != tc.val {
				t.Errorf("round-trip = %d, want %d", parsed, tc.val)
			}
		})
	}
}

// TestOneHourRangeRejectsOutOfRange asserts that a value outside sep.xsd's
// OneHourRangeType range (-3600 to 3600) is refused rather than silently
// serialized or decoded. int16 alone permits -32768..32767, which is wider
// than the schema's facet-restricted range, so the type's own encode/decode
// methods carry the guard.
func TestOneHourRangeRejectsOutOfRange(t *testing.T) {
	cases := []sep2.OneHourRange{3601, -3601, 32767, -32768}
	for _, v := range cases {
		if _, err := xml.Marshal(v); err == nil {
			t.Errorf("Marshal(%d) succeeded, want a refusal outside [-3600, 3600]", v)
		}
	}

	for _, text := range []string{"3601", "-3601", "40000"} {
		doc := "<value>" + text + "</value>"
		var parsed sep2.OneHourRange
		if err := xml.Unmarshal([]byte(doc), &parsed); err == nil {
			t.Errorf("Unmarshal(%q) succeeded, want a refusal outside [-3600, 3600]", text)
		}
	}
}

// TestRandomizableEventFieldsAreOneHourRange asserts RandomizeDuration and
// RandomizeStart are OneHourRange (backed by int16, range-guarded), not the
// former int32, per #111.
func TestRandomizableEventFieldsAreOneHourRange(t *testing.T) {
	var e sep2.RandomizableEvent
	var rs, rd *sep2.OneHourRange
	e.RandomizeStart = rs
	e.RandomizeDuration = rd
	_ = e
}
