package sep2_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

const quantityNS = `xmlns="urn:ieee:std:2030.5:ns"`

// quantityCase checks one {multiplier, value} type through its decoder.
type quantityCase struct {
	name   string
	decode func(doc string) (mult int8, value int64, err error)
}

func decodeAs[T any](get func(T) (int8, int64)) func(string) (int8, int64, error) {
	return func(doc string) (int8, int64, error) {
		var v T
		if err := xml.Unmarshal([]byte(doc), &v); err != nil {
			return 0, 0, err
		}
		m, val := get(v)
		return m, val, nil
	}
}

func quantityCases() []quantityCase {
	return []quantityCase{
		{"ActivePower", decodeAs(func(q sep2.ActivePower) (int8, int64) { return q.Multiplier, int64(q.Value) })},
		{"ReactivePower", decodeAs(func(q sep2.ReactivePower) (int8, int64) { return q.Multiplier, int64(q.Value) })},
		{"ApparentPower", decodeAs(func(q sep2.ApparentPower) (int8, int64) { return q.Multiplier, int64(q.Value) })},
		{"VoltageRMS", decodeAs(func(q sep2.VoltageRMS) (int8, int64) { return q.Multiplier, int64(q.Value) })},
		{"CurrentRMS", decodeAs(func(q sep2.CurrentRMS) (int8, int64) { return q.Multiplier, int64(q.Value) })},
		{"AmpereHour", decodeAs(func(q sep2.AmpereHour) (int8, int64) { return q.Multiplier, int64(q.Value) })},
		{"WattHour", decodeAs(func(q sep2.WattHour) (int8, int64) { return q.Multiplier, int64(q.Value) })},
		{"ReactiveSusceptance", decodeAs(func(q sep2.ReactiveSusceptance) (int8, int64) { return q.Multiplier, int64(q.Value) })},
		{"RealEnergy", decodeAs(func(q sep2.RealEnergy) (int8, int64) { return q.Multiplier, q.Value })},
		{"SignedRealEnergy", decodeAs(func(q sep2.SignedRealEnergy) (int8, int64) { return q.Multiplier, q.Value })},
		{"Temperature", decodeAs(func(q sep2.Temperature) (int8, int64) { return q.Multiplier, int64(q.Value) })},
	}
}

func TestQuantityBareIntegerRefused(t *testing.T) {
	for _, c := range quantityCases() {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := c.decode(`<rating ` + quantityNS + `>16</rating>`)
			if err == nil {
				t.Fatal("bare integer decoded without error")
			}
			if !strings.Contains(err.Error(), c.name) || !strings.Contains(err.Error(), "<rating>") {
				t.Fatalf("error %q does not name the type %s and element <rating>", err, c.name)
			}
		})
	}
}

func TestQuantityComplexFormDecodes(t *testing.T) {
	for _, c := range quantityCases() {
		t.Run(c.name, func(t *testing.T) {
			doc := `<rating ` + quantityNS + `>
  <multiplier>-1</multiplier>
  <value>16</value>
</rating>`
			m, v, err := c.decode(doc)
			if err != nil {
				t.Fatal(err)
			}
			if m != -1 || v != 16 {
				t.Fatalf("got multiplier %d value %d, want -1 and 16", m, v)
			}
		})
	}
}

// An empty element keeps its pre-existing behavior: it decodes to the zero
// value without error.
func TestQuantityEmptyElementUnchanged(t *testing.T) {
	for _, c := range quantityCases() {
		t.Run(c.name, func(t *testing.T) {
			m, v, err := c.decode(`<rating ` + quantityNS + `/>`)
			if err != nil {
				t.Fatal(err)
			}
			if m != 0 || v != 0 {
				t.Fatalf("got multiplier %d value %d, want zero", m, v)
			}
		})
	}
}

func TestDERCapabilityBareIntegerRatingFailsDecode(t *testing.T) {
	doc := `<DERCapability ` + quantityNS + `><rtgMaxW><multiplier>0</multiplier><value>5000</value></rtgMaxW><rtgMaxA>16</rtgMaxA></DERCapability>`
	var c sep2.DERCapability
	err := xml.Unmarshal([]byte(doc), &c)
	if err == nil {
		t.Fatalf("decoded with RTGMaxA %+v, want an error", c.RTGMaxA)
	}
	if !strings.Contains(err.Error(), "rtgMaxA") {
		t.Fatalf("error %q does not name rtgMaxA", err)
	}
}

// pfCase checks one {displacement, multiplier} type through its decoder.
type pfCase struct {
	name    string
	complex string
	decode  func(doc string) (displacement uint16, multiplier int8, err error)
}

func pfCases() []pfCase {
	return []pfCase{
		{"PowerFactor", `<displacement>9500</displacement><multiplier>-2</multiplier>`, func(doc string) (uint16, int8, error) {
			var v sep2.PowerFactor
			err := xml.Unmarshal([]byte(doc), &v)
			return v.Displacement, v.Multiplier, err
		}},
		{"FixedPowerFactor", `<displacement>9500</displacement><excitation>true</excitation><multiplier>-2</multiplier>`, func(doc string) (uint16, int8, error) {
			var v sep2.FixedPowerFactor
			err := xml.Unmarshal([]byte(doc), &v)
			return v.Displacement, v.Multiplier, err
		}},
	}
}

func TestPowerFactorBareIntegerRefused(t *testing.T) {
	for _, c := range pfCases() {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := c.decode(`<pf ` + quantityNS + `>95</pf>`)
			if err == nil {
				t.Fatal("bare integer decoded without error")
			}
			if !strings.Contains(err.Error(), c.name) || !strings.Contains(err.Error(), "<pf>") {
				t.Fatalf("error %q does not name the type %s and element <pf>", err, c.name)
			}
		})
	}
}

func TestPowerFactorComplexAndEmpty(t *testing.T) {
	for _, c := range pfCases() {
		t.Run(c.name, func(t *testing.T) {
			d, m, err := c.decode(`<pf ` + quantityNS + `>` + c.complex + `</pf>`)
			if err != nil || d != 9500 || m != -2 {
				t.Fatalf("complex form: displacement %d multiplier %d err %v, want 9500, -2, nil", d, m, err)
			}
			d, m, err = c.decode(`<pf ` + quantityNS + `/>`)
			if err != nil || d != 0 || m != 0 {
				t.Fatalf("empty element: displacement %d multiplier %d err %v, want zero, nil", d, m, err)
			}
		})
	}
}

// quantityShapeCases are content shapes beyond the bare integer. wantErr is
// empty where the decode must still succeed (today's behavior).
func TestQuantityContentShapes(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr string
		wantVal int64
	}{
		{"nested element in value", `<multiplier>0</multiplier><value><x>5</x></value>`, "<value>: unexpected child element <x>", 0},
		{"nested element in multiplier", `<multiplier><x/></multiplier><value>5</value>`, "<multiplier>: unexpected child element <x>", 0},
		{"text after the children", `<multiplier>0</multiplier><value>5</value>stray`, `unexpected text "stray"`, 0},
		{"text before the children", `stray<multiplier>0</multiplier><value>5</value>`, `unexpected text "stray"`, 0},
		{"comment only", `<!-- 16 -->`, "", 0},
		{"comment between children", `<multiplier>0</multiplier><!-- c --><value>5</value>`, "", 5},
		{"CDATA bare integer", `<![CDATA[16]]>`, `unexpected text "16"`, 0},
		{"unknown element with children", `<ext><y>1</y></ext><value>5</value>`, "", 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var q sep2.CurrentRMS
			err := xml.Unmarshal([]byte(`<rating `+quantityNS+`>`+c.body+`</rating>`), &q)
			if c.wantErr == "" {
				if err != nil || int64(q.Value) != c.wantVal {
					t.Fatalf("got value %d err %v, want value %d and nil", q.Value, err, c.wantVal)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) || !strings.Contains(err.Error(), "<rating>") {
				t.Fatalf("err %v, want it to contain %q and name <rating>", err, c.wantErr)
			}
			if q.Value != 0 {
				t.Fatalf("value %d stored despite the refusal", q.Value)
			}
		})
	}
}

func TestBoundedElementRefusesMixedChild(t *testing.T) {
	var p sep2.PerCent
	err := xml.Unmarshal([]byte(`<v `+quantityNS+`><x/>5</v>`), &p)
	if err == nil || !strings.Contains(err.Error(), "PerCent: unexpected child element <x>") || p != 0 {
		t.Fatalf("got %d, %v, want a refusal naming <x>", p, err)
	}
}

func TestPeerTextQuoteIsBounded(t *testing.T) {
	long := strings.Repeat("a", 100)
	cases := map[string]error{}
	var q sep2.CurrentRMS
	cases["quantity"] = xml.Unmarshal([]byte(`<r `+quantityNS+`>`+long+`</r>`), &q)
	var f sep2.FixedVar
	cases["complex element"] = xml.Unmarshal([]byte(`<r `+quantityNS+`>`+long+`</r>`), &f)
	for name, err := range cases {
		if err == nil {
			t.Fatalf("%s: want an error", name)
		}
		want := `"` + strings.Repeat("a", 32) + `"...`
		if !strings.Contains(err.Error(), want) || strings.Contains(err.Error(), strings.Repeat("a", 33)) {
			t.Errorf("%s: error %q, want the text cut to 32 bytes then ...", name, err)
		}
	}
	var short sep2.CurrentRMS
	err := xml.Unmarshal([]byte(`<r `+quantityNS+`>16</r>`), &short)
	if err == nil || !strings.Contains(err.Error(), `"16"`) || strings.Contains(err.Error(), `"..."`) {
		t.Errorf("short text: %v, want it quoted whole with no ellipsis", err)
	}
}
