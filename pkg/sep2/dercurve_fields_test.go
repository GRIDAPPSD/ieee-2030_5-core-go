package sep2_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// derCurveDoc sets every element sep2.DERCurve models to a non-zero value, in
// sep.xsd sequence order.
const derCurveDoc = `<DERCurve xmlns="urn:ieee:std:2030.5:ns" href="/derp/0/dc/3">` +
	`<mRID>0102030405060708090A0B0C0D0E0F10</mRID>` +
	`<description>volt-var</description>` +
	`<creationTime>1341446380</creationTime>` +
	`<CurveData><xvalue>99</xvalue><yvalue>50</yvalue></CurveData>` +
	`<CurveData><xvalue>103</xvalue><yvalue>-50</yvalue></CurveData>` +
	`<curveType>11</curveType>` +
	`<rampDecTms>600</rampDecTms>` +
	`<rampIncTms>500</rampIncTms>` +
	`<rampPT1Tms>10</rampPT1Tms>` +
	`<xMultiplier>-1</xMultiplier>` +
	`<yMultiplier>2</yMultiplier>` +
	`<yRefType>3</yRefType>` +
	`</DERCurve>`

func decodeDERCurveDoc(t *testing.T) sep2.DERCurve {
	t.Helper()
	var c sep2.DERCurve
	if err := xml.Unmarshal([]byte(derCurveDoc), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return c
}

func TestDERCurveZeroValueEmitsRequiredElements(t *testing.T) {
	data, err := xml.Marshal(sep2.DERCurve{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(data)
	for _, want := range []string{
		"<creationTime>0</creationTime>",
		"<curveType>0</curveType>",
		"<xMultiplier>0</xMultiplier>",
		"<yMultiplier>0</yMultiplier>",
		"<yRefType>0</yRefType>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("zero-value DERCurve lacks %s\nXML: %s", want, got)
		}
	}
}

func TestDERCurveDecodesRequiredElements(t *testing.T) {
	c := decodeDERCurveDoc(t)
	if c.CreationTime != 1341446380 {
		t.Errorf("CreationTime = %d, want 1341446380", c.CreationTime)
	}
	if c.XMultiplier != -1 {
		t.Errorf("XMultiplier = %d, want -1", c.XMultiplier)
	}
	if c.YMultiplier != 2 {
		t.Errorf("YMultiplier = %d, want 2", c.YMultiplier)
	}
	if c.YRefType != 3 {
		t.Errorf("YRefType = %d, want 3", c.YRefType)
	}
	if c.CurveType != 11 || len(c.CurveData) != 2 {
		t.Errorf("CurveType = %d with %d points, want 11 with 2", c.CurveType, len(c.CurveData))
	}
}

func TestDERCurveRequiredElementsSurviveDecodeEncode(t *testing.T) {
	data, err := xml.Marshal(decodeDERCurveDoc(t))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(data)
	for _, want := range []string{
		"<creationTime>1341446380</creationTime>",
		"<xMultiplier>-1</xMultiplier>",
		"<yMultiplier>2</yMultiplier>",
		"<yRefType>3</yRefType>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("re-encoded DERCurve lacks %s\nXML: %s", want, got)
		}
	}
}

// TestDERCurveWireOrder asserts the sep.xsd DERCurve sequence for every
// element the struct models. The two chains leave out CurveData against
// curveType, which the struct emits in the opposite order to sep.xsd; the
// schema gate pins that as a known failure.
func TestDERCurveWireOrder(t *testing.T) {
	data, err := xml.Marshal(decodeDERCurveDoc(t))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	xmlStr := string(data)

	assertOrder(t, xmlStr, []string{
		"<mRID>",
		"<description>",
		"<creationTime>",
		"<CurveData>",
		"<rampDecTms>",
		"<rampIncTms>",
		"<rampPT1Tms>",
		"<xMultiplier>",
		"<yMultiplier>",
		"<yRefType>",
	})
	assertOrder(t, xmlStr, []string{
		"<creationTime>",
		"<curveType>",
		"<rampDecTms>",
	})
}

func TestDERCurveCopyKeepsRequiredElements(t *testing.T) {
	original := decodeDERCurveDoc(t)
	copied := original.Copy()

	if copied.CreationTime != 1341446380 || copied.XMultiplier != -1 ||
		copied.YMultiplier != 2 || copied.YRefType != 3 {
		t.Errorf("Copy lost a required element: %+v", copied)
	}

	copied.CreationTime = 1
	copied.XMultiplier = 0
	copied.YRefType = 0
	if original.CreationTime != 1341446380 || original.XMultiplier != -1 || original.YRefType != 3 {
		t.Errorf("mutating the copy changed the original: %+v", original)
	}
}
