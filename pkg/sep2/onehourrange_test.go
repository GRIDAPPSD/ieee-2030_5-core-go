package sep2_test

import (
	"encoding/json"
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

// TestOneHourRangeAcceptsCollapsedWhitespace pins xs:short's whitespace
// collapse: surrounding whitespace is part of a valid lexical form.
func TestOneHourRangeAcceptsCollapsedWhitespace(t *testing.T) {
	cases := []struct {
		text string
		want sep2.OneHourRange
	}{
		{" 3600 ", 3600},
		{"\n-3600\t", -3600},
		{"\r\n 30 \r\n", 30},
	}
	for _, tc := range cases {
		var parsed sep2.OneHourRange
		if err := xml.Unmarshal([]byte("<value>"+tc.text+"</value>"), &parsed); err != nil {
			t.Errorf("Unmarshal(%q): %v", tc.text, err)
			continue
		}
		if parsed != tc.want {
			t.Errorf("Unmarshal(%q) = %d, want %d", tc.text, parsed, tc.want)
		}
	}

	var ev sep2.DERControl
	doc := `<DERControl xmlns="urn:ieee:std:2030.5:ns"><randomizeStart> -30 </randomizeStart></DERControl>`
	if err := xml.Unmarshal([]byte(doc), &ev); err != nil {
		t.Fatalf("Unmarshal DERControl: %v", err)
	}
	if ev.RandomizeStart == nil || *ev.RandomizeStart != -30 {
		t.Errorf("DERControl.RandomizeStart = %v, want -30", ev.RandomizeStart)
	}
}

// TestOneHourRangeJSONRange asserts the JSON path carries the same range
// guard as XML, since server-go ingests JSON fixtures for this package.
func TestOneHourRangeJSONRange(t *testing.T) {
	accept := []struct {
		text string
		want sep2.OneHourRange
	}{
		{"3600", 3600},
		{"-3600", -3600},
		{"0", 0},
	}
	for _, tc := range accept {
		var v sep2.OneHourRange
		if err := json.Unmarshal([]byte(tc.text), &v); err != nil {
			t.Errorf("json.Unmarshal(%s): %v", tc.text, err)
			continue
		}
		if v != tc.want {
			t.Errorf("json.Unmarshal(%s) = %d, want %d", tc.text, v, tc.want)
		}
		out, err := json.Marshal(v)
		if err != nil {
			t.Errorf("json.Marshal(%d): %v", v, err)
			continue
		}
		if string(out) != tc.text {
			t.Errorf("json.Marshal(%d) = %s, want %s", v, out, tc.text)
		}
	}

	for _, text := range []string{"3601", "-3601", "5000", "40000", "-32768", `"12"`, "1.5"} {
		var v sep2.OneHourRange
		if err := json.Unmarshal([]byte(text), &v); err == nil {
			t.Errorf("json.Unmarshal(%s) = %d, want a refusal outside [-3600, 3600]", text, v)
		}
	}
	for _, v := range []sep2.OneHourRange{3601, -3601, 4000, 32767, -32768} {
		if out, err := json.Marshal(v); err == nil {
			t.Errorf("json.Marshal(%d) = %s, want a refusal outside [-3600, 3600]", v, out)
		}
	}

	var ev sep2.RandomizableEvent
	if err := json.Unmarshal([]byte(`{"RandomizeStart":5000}`), &ev); err == nil {
		t.Errorf("RandomizableEvent decoded RandomizeStart 5000 from JSON, want a refusal")
	}
	start := sep2.OneHourRange(4000)
	if out, err := json.Marshal(sep2.RandomizableEvent{RandomizeStart: &start}); err == nil {
		t.Errorf("json.Marshal(RandomizableEvent{RandomizeStart: 4000}) = %s, want a refusal", out)
	}
	ev = sep2.RandomizableEvent{}
	if err := json.Unmarshal([]byte(`{"RandomizeStart":-3600,"RandomizeDuration":null}`), &ev); err != nil {
		t.Fatalf("json.Unmarshal in-range event: %v", err)
	}
	if ev.RandomizeStart == nil || *ev.RandomizeStart != -3600 || ev.RandomizeDuration != nil {
		t.Errorf("decoded RandomizeStart=%v RandomizeDuration=%v, want -3600 and nil", ev.RandomizeStart, ev.RandomizeDuration)
	}
}
