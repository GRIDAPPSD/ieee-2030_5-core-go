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
