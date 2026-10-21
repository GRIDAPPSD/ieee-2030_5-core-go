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
// decode rather than yield a zero value, and with the error of its own guard.
func TestDERControlRejectsMalformedModeElements(t *testing.T) {
	t.Parallel()
	const droopTail = `<dBUF>37</dBUF><kOF>50</kOF><kUF>40</kUF><openLoopTms>500</openLoopTms>`
	const droopOK = `<dBOF>36</dBOF>` + droopTail
	droop := func(inner string) string { return `<opModFreqDroop>` + inner + `</opModFreqDroop>` }
	fixedVar := func(inner string) string { return `<opModFixedVar>` + inner + `</opModFixedVar>` }
	type reject struct{ elem, wantErr string }
	cases := map[string]reject{
		"old FixedVar multiplier and value":  {fixedVar(`<multiplier>0</multiplier><value>5</value>`), "FixedVar: unexpected element <multiplier>"},
		"FixedVar missing refType":           {fixedVar(`<value>5</value>`), "FixedVar: unexpected element <value>"},
		"FixedVar missing value":             {fixedVar(`<refType>1</refType>`), "FixedVar: missing element <value>"},
		"FixedVar repeated value":            {fixedVar(`<refType>1</refType><value>5</value><value>6</value>`), "FixedVar: unexpected element <value>"},
		"FixedVar out of order":              {fixedVar(`<value>5</value><refType>1</refType>`), "FixedVar: unexpected element <value>"},
		"FixedVar refType over UInt8":        {fixedVar(`<refType>256</refType><value>5</value>`), `FixedVar.refType: strconv.ParseUint: parsing "256": value out of range`},
		"FixedVar refType 0":                 {fixedVar(`<refType>0</refType><value>5</value>`), "FixedVar: refType 0 is not"},
		"FixedVar refType 4":                 {fixedVar(`<refType>4</refType><value>5</value>`), "FixedVar: refType 4 is not"},
		"FixedVar value over SignedPerCent":  {fixedVar(`<refType>1</refType><value>10001</value>`), "SignedPerCent 10001 out of range"},
		"FixedVar empty value":               {fixedVar(`<refType>1</refType><value></value>`), "SignedPerCent: empty or non-scalar element"},
		"FixedVar bare text":                 {fixedVar(`5`), "FixedVar: unexpected text"},
		"FixedVar refType in another space":  {fixedVar(`<o:refType xmlns:o="urn:other">1</o:refType><value>5</value>`), `FixedVar: element <refType> in namespace "urn:other"`},
		"FixedVar value with nested element": {fixedVar(`<refType>1</refType><value><x/>5</value>`), "FixedVar.value: unexpected child element <x>"},
		"old FreqDroop scalar":               {droop(`55`), "FreqDroopType: unexpected text"},
		"FreqDroop missing openLoopTms":      {droop(`<dBOF>36</dBOF><dBUF>37</dBUF><kOF>50</kOF><kUF>40</kUF>`), "FreqDroopType: missing element <openLoopTms>"},
		"FreqDroop unknown child":            {droop(droopOK + `<extra>1</extra>`), "FreqDroopType: unexpected element <extra>"},
		"FreqDroop kOF over UInt16":          {droop(`<dBOF>36</dBOF><dBUF>37</dBUF><kOF>65536</kOF><kUF>40</kUF><openLoopTms>500</openLoopTms>`), `FreqDroopType.kOF: strconv.ParseUint: parsing "65536": value out of range`},
		"FreqDroop dBOF over UInt32":         {droop(`<dBOF>4294967296</dBOF>` + droopTail), `FreqDroopType.dBOF: strconv.ParseUint: parsing "4294967296": value out of range`},
		"FreqDroop dBUF over UInt32":         {droop(`<dBOF>36</dBOF><dBUF>4294967296</dBUF><kOF>50</kOF><kUF>40</kUF><openLoopTms>500</openLoopTms>`), `FreqDroopType.dBUF: strconv.ParseUint: parsing "4294967296": value out of range`},
		"FreqDroop negative dBOF":            {droop(`<dBOF>-1</dBOF>` + droopTail), `FreqDroopType.dBOF: strconv.ParseUint: parsing "-1": invalid syntax`},
		"FreqDroop text beside children":     {droop(`9` + droopOK), "FreqDroopType: unexpected text"},
		"FreqDroop dBOF with nested element": {droop(`<dBOF><x/>1</dBOF>` + droopTail), "FreqDroopType.dBOF: unexpected child element <x>"},
		"FreqDroop kOF in another space":     {droop(`<dBOF>36</dBOF><dBUF>37</dBUF><o:kOF xmlns:o="urn:other">50</o:kOF><kUF>40</kUF><openLoopTms>500</openLoopTms>`), `FreqDroopType: element <kOF> in namespace "urn:other"`},
		"curve link with empty href":         {`<opModVoltVar href=""></opModVoltVar>`, "opModVoltVar: DERCurveLink: href is required"},
		"curve link with blank href":         {`<opModVoltWatt href="  "></opModVoltWatt>`, "opModVoltWatt: DERCurveLink: href is required"},
		"curve link with child element":      {`<opModVoltVar href="/dc/1"><x/></opModVoltVar>`, "DERCurveLink: unexpected element <x>"},
		"curve link with href and old text":  {`<opModVoltVar href="/dc/1">3</opModVoltVar>`, "DERCurveLink: unexpected text"},
		"curve link href in another space":   {`<opModVoltVar xmlns:o="urn:other" o:href="/dc/1"></opModVoltVar>`, "opModVoltVar: DERCurveLink: href is required"},
	}
	for _, f := range curveLinkFields() {
		cases["old int32 "+f.element] = reject{`<` + f.element + `>3</` + f.element + `>`, f.element + ": DERCurveLink: href is required"}
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			doc := `<DERControl xmlns="urn:ieee:std:2030.5:ns"><DERControlBase>` + tc.elem + `</DERControlBase></DERControl>`
			var parsed sep2.DERControl
			err := xml.Unmarshal([]byte(doc), &parsed)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %v, want one containing %q (decoded %+v)", err, tc.wantErr, parsed.DERControlBase)
			}
		})
	}
}

// TestDERControlModeElementBoundaries pins the accepted edges: the UInt32
// maximum for the droop deadbands, the refType range 1 to 3, and an href
// whose surrounding whitespace the schema's anyURI collapse removes.
func TestDERControlModeElementBoundaries(t *testing.T) {
	t.Parallel()
	const maxU32 = 4294967295
	doc := `<DERControl xmlns="urn:ieee:std:2030.5:ns"><DERControlBase>` +
		`<opModFixedVar><refType>3</refType><value>1</value></opModFixedVar>` +
		`<opModFreqDroop><dBOF>4294967295</dBOF><dBUF>4294967295</dBUF><kOF>1</kOF><kUF>2</kUF><openLoopTms>3</openLoopTms></opModFreqDroop>` +
		`<opModVoltVar href="  /dc/1  "></opModVoltVar>` +
		`</DERControlBase></DERControl>`
	base := unmarshalControl(t, []byte(doc))
	if base.OpModFreqDroop == nil || base.OpModFreqDroop.DBOF != maxU32 || base.OpModFreqDroop.DBUF != maxU32 {
		t.Errorf("droop at UInt32 max = %+v", base.OpModFreqDroop)
	}
	if base.OpModFixedVar == nil || base.OpModFixedVar.RefType != 3 {
		t.Errorf("refType 3 = %+v", base.OpModFixedVar)
	}
	if base.OpModVoltVar == nil || base.OpModVoltVar.Href != "/dc/1" {
		t.Errorf("padded href decoded as %+v, want /dc/1", base.OpModVoltVar)
	}
	if data := marshalControl(t, base); !strings.Contains(string(data), `<opModVoltVar href="/dc/1">`) {
		t.Errorf("padded href re-marshalled with padding: %s", data)
	}

	var fromJSON sep2.DERControlBase
	err := json.Unmarshal([]byte(`{"OpModFixedVar":{"RefType":1,"Value":1},`+
		`"OpModFreqDroop":{"DBOF":4294967295,"DBUF":4294967295,"KOF":1,"KUF":2,"OpenLoopTms":3},`+
		`"OpModVoltVar":{"Href":" /dc/2 "}}`), &fromJSON)
	if err != nil {
		t.Fatalf("JSON at the bounds: %v", err)
	}
	if fromJSON.OpModFreqDroop.DBOF != maxU32 || fromJSON.OpModFreqDroop.DBUF != maxU32 ||
		fromJSON.OpModFixedVar.RefType != 1 || fromJSON.OpModVoltVar.Href != "/dc/2" {
		t.Errorf("JSON at the bounds = %+v %+v %+v", fromJSON.OpModFreqDroop, fromJSON.OpModFixedVar, fromJSON.OpModVoltVar)
	}
}

func TestDERControlRefusesToMarshalInvalidModes(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		base    sep2.DERControlBase
		wantErr string
	}{
		"empty href":  {sep2.DERControlBase{OpModVoltVar: &sep2.DERCurveLink{Href: ""}}, "DERCurveLink: href is required"},
		"blank href":  {sep2.DERControlBase{OpModVoltVar: &sep2.DERCurveLink{Href: " "}}, "DERCurveLink: href is required"},
		"refType 0":   {sep2.DERControlBase{OpModFixedVar: &sep2.FixedVar{RefType: 0, Value: 1}}, "FixedVar: refType 0 is not"},
		"refType 255": {sep2.DERControlBase{OpModFixedVar: &sep2.FixedVar{RefType: 255, Value: 1}}, "FixedVar: refType 255 is not"},
	}
	for name, tc := range cases {
		base := tc.base
		if data, err := xml.Marshal(&sep2.DERControl{DERControlBase: &base}); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: XML error = %v, want %q (wrote %s)", name, err, tc.wantErr, data)
		}
		if data, err := json.Marshal(base); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: JSON error = %v, want %q (wrote %s)", name, err, tc.wantErr, data)
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

	rejected := map[string]struct{ doc, wantErr string }{
		"old curve ref integer":       {`{"OpModVoltVar":3}`, "DERCurveLink: json: cannot unmarshal number"},
		"curve link without href":     {`{"OpModVoltVar":{}}`, "DERCurveLink: href is required"},
		"curve link with empty href":  {`{"OpModVoltVar":{"Href":""}}`, "DERCurveLink: href is required"},
		"curve link with blank href":  {`{"OpModVoltVar":{"Href":"  "}}`, "DERCurveLink: href is required"},
		"curve link with unknown key": {`{"OpModVoltVar":{"Href":"/dc/1","ID":3}}`, `unknown field "ID"`},
		"old FreqDroop integer":       {`{"OpModFreqDroop":55}`, "FreqDroopType: json: cannot unmarshal number"},
		"FreqDroop missing DBOF":      {`{"OpModFreqDroop":{"DBUF":37,"KOF":50,"KUF":40,"OpenLoopTms":500}}`, "FreqDroopType: DBOF is required"},
		"FreqDroop missing DBUF":      {`{"OpModFreqDroop":{"DBOF":36,"KOF":50,"KUF":40,"OpenLoopTms":500}}`, "FreqDroopType: DBUF is required"},
		"FreqDroop missing KOF":       {`{"OpModFreqDroop":{"DBOF":36,"DBUF":37,"KUF":40,"OpenLoopTms":500}}`, "FreqDroopType: KOF is required"},
		"FreqDroop missing KUF":       {`{"OpModFreqDroop":{"DBOF":36,"DBUF":37,"KOF":50,"OpenLoopTms":500}}`, "FreqDroopType: KUF is required"},
		"FreqDroop missing OpenLoop":  {`{"OpModFreqDroop":{"DBOF":36,"DBUF":37,"KOF":50,"KUF":40}}`, "FreqDroopType: OpenLoopTms is required"},
		"FreqDroop DBOF over UInt32":  {`{"OpModFreqDroop":{"DBOF":4294967296,"DBUF":37,"KOF":50,"KUF":40,"OpenLoopTms":500}}`, "FreqDroopType: json: cannot unmarshal number 4294967296"},
		"FreqDroop DBUF over UInt32":  {`{"OpModFreqDroop":{"DBOF":36,"DBUF":4294967296,"KOF":50,"KUF":40,"OpenLoopTms":500}}`, "FreqDroopType: json: cannot unmarshal number 4294967296"},
		"old FixedVar ReactivePower":  {`{"OpModFixedVar":{"Multiplier":0,"Value":5}}`, `unknown field "Multiplier"`},
		"FixedVar missing RefType":    {`{"OpModFixedVar":{"Value":5}}`, "FixedVar: RefType is required"},
		"FixedVar missing Value":      {`{"OpModFixedVar":{"RefType":1}}`, "FixedVar: Value is required"},
		"FixedVar value out of range": {`{"OpModFixedVar":{"RefType":1,"Value":10001}}`, "SignedPerCent 10001 out of range"},
		"FixedVar refType 0":          {`{"OpModFixedVar":{"RefType":0,"Value":5}}`, "FixedVar: refType 0 is not"},
		"FixedVar refType 4":          {`{"OpModFixedVar":{"RefType":4,"Value":5}}`, "FixedVar: refType 4 is not"},
	}
	for name, tc := range rejected {
		var b sep2.DERControlBase
		if err := json.Unmarshal([]byte(tc.doc), &b); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: error = %v, want one containing %q (decoded %+v)", name, err, tc.wantErr, b)
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
