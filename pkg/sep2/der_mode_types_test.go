package sep2_test

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// curveLinkField names one DERControlBase field typed DERCurveLink.
type curveLinkField struct {
	element string
	field   func(*sep2.DERControlBase) **sep2.DERCurveLink
}

// curveLinkFields lists the nine curve-reference fields of #153 and #122.
func curveLinkFields() []curveLinkField {
	return []curveLinkField{
		{"opModFreqWatt", func(b *sep2.DERControlBase) **sep2.DERCurveLink { return &b.OpModFreqWatt }},
		{"opModHFRTMustTrip", func(b *sep2.DERControlBase) **sep2.DERCurveLink { return &b.OpModHFRTMustTrip }},
		{"opModHVRTMomentaryCessation", func(b *sep2.DERControlBase) **sep2.DERCurveLink { return &b.OpModHVRTMomentaryCessation }},
		{"opModHVRTMustTrip", func(b *sep2.DERControlBase) **sep2.DERCurveLink { return &b.OpModHVRTMustTrip }},
		{"opModLFRTMustTrip", func(b *sep2.DERControlBase) **sep2.DERCurveLink { return &b.OpModLFRTMustTrip }},
		{"opModLVRTMomentaryCessation", func(b *sep2.DERControlBase) **sep2.DERCurveLink { return &b.OpModLVRTMomentaryCessation }},
		{"opModLVRTMustTrip", func(b *sep2.DERControlBase) **sep2.DERCurveLink { return &b.OpModLVRTMustTrip }},
		{"opModVoltVar", func(b *sep2.DERControlBase) **sep2.DERCurveLink { return &b.OpModVoltVar }},
		{"opModVoltWatt", func(b *sep2.DERControlBase) **sep2.DERCurveLink { return &b.OpModVoltWatt }},
	}
}

// childTexts returns the text of each direct child of the first element
// named parent, in document order, read from the token stream rather than
// by substring so a misplaced or nested child cannot pass.
func childTexts(t *testing.T, data []byte, parent string) [][2]string {
	t.Helper()
	d := xml.NewDecoder(bytes.NewReader(data))
	depth, inParent := 0, false
	var out [][2]string
	var cur string
	var text strings.Builder
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch tt := tok.(type) {
		case xml.StartElement:
			depth++
			if !inParent && tt.Name.Local == parent {
				inParent, depth = true, 0
				continue
			}
			if inParent && depth == 1 {
				cur = tt.Name.Local
				text.Reset()
			}
			if inParent && depth > 1 {
				t.Errorf("<%s> has a grandchild <%s>", parent, tt.Name.Local)
			}
		case xml.CharData:
			if inParent && depth == 1 {
				text.Write(tt)
			}
		case xml.EndElement:
			if inParent && depth == 0 {
				return out
			}
			if inParent && depth == 1 {
				out = append(out, [2]string{cur, text.String()})
			}
			depth--
		}
	}
	t.Fatalf("no <%s> element in %s", parent, data)
	return nil
}

func marshalControl(t *testing.T, base *sep2.DERControlBase) []byte {
	t.Helper()
	data, err := xml.Marshal(&sep2.DERControl{DERControlBase: base})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

func unmarshalControl(t *testing.T, data []byte) *sep2.DERControlBase {
	t.Helper()
	var parsed sep2.DERControl
	if err := xml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v\nXML: %s", err, data)
	}
	if parsed.DERControlBase == nil {
		t.Fatalf("parsed DERControlBase is nil\nXML: %s", data)
	}
	return parsed.DERControlBase
}

func TestDERControlCurveLinkWireForm(t *testing.T) {
	t.Parallel()
	for i, f := range curveLinkFields() {
		t.Run(f.element, func(t *testing.T) {
			t.Parallel()
			href := "/dc/" + string(rune('1'+i))
			base := &sep2.DERControlBase{}
			*f.field(base) = &sep2.DERCurveLink{Href: href}

			data := marshalControl(t, base)
			wantElem := `<` + f.element + ` href="` + href + `"></` + f.element + `>`
			if !strings.Contains(string(data), wantElem) {
				t.Errorf("wire form missing %s\nXML: %s", wantElem, data)
			}
			if kids := childTexts(t, data, f.element); len(kids) != 0 {
				t.Errorf("<%s> carries children %v; a link has none", f.element, kids)
			}

			parsed := unmarshalControl(t, data)
			got := *f.field(parsed)
			if got == nil || got.Href != href {
				t.Fatalf("decoded %s = %+v, want href %q", f.element, got, href)
			}
			for _, other := range curveLinkFields() {
				if other.element != f.element && *other.field(parsed) != nil {
					t.Errorf("decoding %s also set %s", f.element, other.element)
				}
			}

			copied := base.Copy()
			(*f.field(&copied)).Href = "/dc/changed"
			if (*f.field(base)).Href != href {
				t.Errorf("Copy shares the %s link with the original", f.element)
			}
		})
	}
}

func TestDERControlFixedVarWireForm(t *testing.T) {
	t.Parallel()
	base := &sep2.DERControlBase{OpModFixedVar: &sep2.FixedVar{RefType: 2, Value: -1500}}

	data := marshalControl(t, base)
	const wantElem = `<opModFixedVar><refType>2</refType><value>-1500</value></opModFixedVar>`
	if !strings.Contains(string(data), wantElem) {
		t.Errorf("wire form missing %s\nXML: %s", wantElem, data)
	}
	want := [][2]string{{"refType", "2"}, {"value", "-1500"}}
	if got := childTexts(t, data, "opModFixedVar"); !equalPairs(got, want) {
		t.Errorf("opModFixedVar children = %v, want %v", got, want)
	}

	parsed := unmarshalControl(t, data)
	if parsed.OpModFixedVar == nil || *parsed.OpModFixedVar != (sep2.FixedVar{RefType: 2, Value: -1500}) {
		t.Errorf("decoded OpModFixedVar = %+v", parsed.OpModFixedVar)
	}

	copied := base.Copy()
	copied.OpModFixedVar.Value = 1
	if base.OpModFixedVar.Value != -1500 {
		t.Error("Copy shares OpModFixedVar with the original")
	}
}

func TestDERControlFreqDroopWireForm(t *testing.T) {
	t.Parallel()
	droop := sep2.FreqDroopType{DBOF: 36, DBUF: 37, KOF: 50, KUF: 40, OpenLoopTms: 500}
	base := &sep2.DERControlBase{OpModFreqDroop: &droop}

	data := marshalControl(t, base)
	const wantElem = `<opModFreqDroop><dBOF>36</dBOF><dBUF>37</dBUF><kOF>50</kOF>` +
		`<kUF>40</kUF><openLoopTms>500</openLoopTms></opModFreqDroop>`
	if !strings.Contains(string(data), wantElem) {
		t.Errorf("wire form missing %s\nXML: %s", wantElem, data)
	}
	want := [][2]string{{"dBOF", "36"}, {"dBUF", "37"}, {"kOF", "50"}, {"kUF", "40"}, {"openLoopTms", "500"}}
	if got := childTexts(t, data, "opModFreqDroop"); !equalPairs(got, want) {
		t.Errorf("opModFreqDroop children = %v, want %v", got, want)
	}

	parsed := unmarshalControl(t, data)
	if parsed.OpModFreqDroop == nil || *parsed.OpModFreqDroop != droop {
		t.Errorf("decoded OpModFreqDroop = %+v, want %+v", parsed.OpModFreqDroop, droop)
	}

	copied := base.Copy()
	copied.OpModFreqDroop.KOF = 1
	if base.OpModFreqDroop.KOF != 50 {
		t.Error("Copy shares OpModFreqDroop with the original")
	}
}

func equalPairs(a, b [][2]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestDERControlRejectsMalformedModeElements covers the pre-#153 wire shapes
// and the ways a new-shape element can be incomplete: each must fail to
// decode rather than yield a zero value.
func TestDERControlRejectsMalformedModeElements(t *testing.T) {
	t.Parallel()
	const droopOK = `<dBOF>36</dBOF><dBUF>37</dBUF><kOF>50</kOF><kUF>40</kUF><openLoopTms>500</openLoopTms>`
	cases := map[string]string{
		"old FixedVar multiplier and value": `<opModFixedVar><multiplier>0</multiplier><value>5</value></opModFixedVar>`,
		"FixedVar missing refType":          `<opModFixedVar><value>5</value></opModFixedVar>`,
		"FixedVar missing value":            `<opModFixedVar><refType>1</refType></opModFixedVar>`,
		"FixedVar repeated value":           `<opModFixedVar><refType>1</refType><value>5</value><value>6</value></opModFixedVar>`,
		"FixedVar out of order":             `<opModFixedVar><value>5</value><refType>1</refType></opModFixedVar>`,
		"FixedVar refType over UInt8":       `<opModFixedVar><refType>256</refType><value>5</value></opModFixedVar>`,
		"FixedVar value over SignedPerCent": `<opModFixedVar><refType>1</refType><value>10001</value></opModFixedVar>`,
		"FixedVar empty value":              `<opModFixedVar><refType>1</refType><value></value></opModFixedVar>`,
		"FixedVar bare text":                `<opModFixedVar>5</opModFixedVar>`,
		"old FreqDroop scalar":              `<opModFreqDroop>55</opModFreqDroop>`,
		"FreqDroop missing openLoopTms":     `<opModFreqDroop><dBOF>36</dBOF><dBUF>37</dBUF><kOF>50</kOF><kUF>40</kUF></opModFreqDroop>`,
		"FreqDroop unknown child":           `<opModFreqDroop>` + droopOK + `<extra>1</extra></opModFreqDroop>`,
		"FreqDroop kOF over UInt16":         `<opModFreqDroop><dBOF>36</dBOF><dBUF>37</dBUF><kOF>65536</kOF><kUF>40</kUF><openLoopTms>500</openLoopTms></opModFreqDroop>`,
		"FreqDroop negative dBOF":           `<opModFreqDroop><dBOF>-1</dBOF><dBUF>37</dBUF><kOF>50</kOF><kUF>40</kUF><openLoopTms>500</openLoopTms></opModFreqDroop>`,
		"FreqDroop text beside children":    `<opModFreqDroop>9` + droopOK + `</opModFreqDroop>`,
		"curve link with empty href":        `<opModVoltVar href=""></opModVoltVar>`,
		"curve link with blank href":        `<opModVoltVar href="  "></opModVoltVar>`,
		"curve link with child element":     `<opModVoltVar href="/dc/1"><x/></opModVoltVar>`,
		"curve link with href and old text": `<opModVoltVar href="/dc/1">3</opModVoltVar>`,
		"curve link href in another space":  `<opModVoltVar xmlns:o="urn:other" o:href="/dc/1"></opModVoltVar>`,
	}
	for _, f := range curveLinkFields() {
		cases["old int32 "+f.element] = `<` + f.element + `>3</` + f.element + `>`
	}
	for name, elem := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			doc := `<DERControl xmlns="urn:ieee:std:2030.5:ns"><DERControlBase>` + elem + `</DERControlBase></DERControl>`
			var parsed sep2.DERControl
			err := xml.Unmarshal([]byte(doc), &parsed)
			if err == nil {
				t.Errorf("decoded without error: %+v", parsed.DERControlBase)
			}
		})
	}
}

func TestDERControlRefusesToMarshalEmptyCurveLink(t *testing.T) {
	t.Parallel()
	for _, href := range []string{"", " "} {
		base := &sep2.DERControlBase{OpModVoltVar: &sep2.DERCurveLink{Href: href}}
		if data, err := xml.Marshal(&sep2.DERControl{DERControlBase: base}); err == nil {
			t.Errorf("href %q marshalled: %s", href, data)
		}
		if data, err := json.Marshal(base); err == nil {
			t.Errorf("href %q marshalled to JSON: %s", href, data)
		}
	}
}

// TestDERControlBaseModeTypesJSON covers the JSON path server-go persists
// controls through: the new shapes round-trip, and a record written in the
// pre-#153 shape fails to load instead of loading as zero.
func TestDERControlBaseModeTypesJSON(t *testing.T) {
	t.Parallel()
	droop := sep2.FreqDroopType{DBOF: 36, DBUF: 37, KOF: 50, KUF: 40, OpenLoopTms: 500}
	in := sep2.DERControlBase{
		OpModFixedVar:  &sep2.FixedVar{RefType: 2, Value: -1500},
		OpModFreqDroop: &droop,
		OpModVoltVar:   &sep2.DERCurveLink{Href: "/dc/7"},
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{
		`"OpModFixedVar":{"RefType":2,"Value":-1500}`,
		`"OpModFreqDroop":{"DBOF":36,"DBUF":37,"KOF":50,"KUF":40,"OpenLoopTms":500}`,
		`"OpModVoltVar":{"Href":"/dc/7"}`,
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("JSON missing %s\nJSON: %s", want, data)
		}
	}
	var out sep2.DERControlBase
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.OpModFixedVar == nil || *out.OpModFixedVar != *in.OpModFixedVar ||
		out.OpModFreqDroop == nil || *out.OpModFreqDroop != droop ||
		out.OpModVoltVar == nil || out.OpModVoltVar.Href != "/dc/7" {
		t.Errorf("JSON round trip = %+v", out)
	}

	rejected := map[string]string{
		"old curve ref integer":       `{"OpModVoltVar":3}`,
		"curve link without href":     `{"OpModVoltVar":{}}`,
		"curve link with empty href":  `{"OpModVoltVar":{"Href":""}}`,
		"curve link with unknown key": `{"OpModVoltVar":{"Href":"/dc/1","ID":3}}`,
		"old FreqDroop integer":       `{"OpModFreqDroop":55}`,
		"FreqDroop missing a field":   `{"OpModFreqDroop":{"DBOF":36,"DBUF":37,"KOF":50,"KUF":40}}`,
		"old FixedVar ReactivePower":  `{"OpModFixedVar":{"Multiplier":0,"Value":5}}`,
		"FixedVar missing RefType":    `{"OpModFixedVar":{"Value":5}}`,
		"FixedVar value out of range": `{"OpModFixedVar":{"RefType":1,"Value":10001}}`,
	}
	for name, doc := range rejected {
		var b sep2.DERControlBase
		if err := json.Unmarshal([]byte(doc), &b); err == nil {
			t.Errorf("%s: decoded without error: %+v", name, b)
		}
	}

	var null sep2.DERControlBase
	if err := json.Unmarshal([]byte(`{"OpModVoltVar":null,"OpModFixedVar":null,"OpModFreqDroop":null}`), &null); err != nil {
		t.Errorf("null modes: %v", err)
	}
	if null.OpModVoltVar != nil || null.OpModFixedVar != nil || null.OpModFreqDroop != nil {
		t.Errorf("null modes decoded as set: %+v", null)
	}
}
