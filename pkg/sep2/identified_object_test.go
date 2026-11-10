package sep2_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

func ioWith(v uint16) sep2.IdentifiedObject {
	return sep2.IdentifiedObject{MRID: "M1", Description: "D1", Version: &v}
}

// identifiedCase holds one embedder: how to marshal it populated, the element
// that must follow version, and how to mutate and read the Version pointer of
// its Copy.
type identifiedCase struct {
	name    string
	marshal func() ([]byte, error)
	next    string
	copyVer func() (orig, cp *uint16)
}

func identifiedCases() []identifiedCase {
	return []identifiedCase{
		{
			name: "DERProgram",
			marshal: func() ([]byte, error) {
				return xml.Marshal(sep2.DERProgram{IdentifiedObject: ioWith(3), Primacy: 1})
			},
			next: "<primacy>",
			copyVer: func() (*uint16, *uint16) {
				p := sep2.DERProgram{IdentifiedObject: ioWith(3)}
				c := p.Copy()
				return p.Version, c.Version
			},
		},
		{
			name: "DERCurve",
			marshal: func() ([]byte, error) {
				return xml.Marshal(sep2.DERCurve{IdentifiedObject: ioWith(3), CreationTime: 5})
			},
			next: "<creationTime>",
			copyVer: func() (*uint16, *uint16) {
				d := sep2.DERCurve{IdentifiedObject: ioWith(3)}
				c := d.Copy()
				return d.Version, c.Version
			},
		},
		{
			name: "MirrorMeterReading",
			marshal: func() ([]byte, error) {
				return xml.Marshal(sep2.MirrorMeterReading{IdentifiedObject: ioWith(3), LastUpdateTime: 9})
			},
			next: "<lastUpdateTime>",
			copyVer: func() (*uint16, *uint16) {
				m := sep2.MirrorMeterReading{IdentifiedObject: ioWith(3)}
				c := m.Copy()
				return m.Version, c.Version
			},
		},
		{
			name: "ReadingSet",
			marshal: func() ([]byte, error) {
				return xml.Marshal(sep2.ReadingSet{IdentifiedObject: ioWith(3)})
			},
			next: "<timePeriod>",
			copyVer: func() (*uint16, *uint16) {
				r := sep2.ReadingSet{IdentifiedObject: ioWith(3)}
				c := r.Copy()
				return r.Version, c.Version
			},
		},
		{
			name: "DERControl",
			marshal: func() ([]byte, error) {
				var d sep2.DERControl
				d.IdentifiedObject = ioWith(3)
				d.CreationTime = 5
				return xml.Marshal(d)
			},
			next: "<creationTime>",
			copyVer: func() (*uint16, *uint16) {
				var d sep2.DERControl
				d.IdentifiedObject = ioWith(3)
				c := d.Copy()
				return d.Version, c.Version
			},
		},
	}
}

func TestIdentifiedObjectWireOrder(t *testing.T) {
	for _, tc := range identifiedCases() {
		t.Run(tc.name, func(t *testing.T) {
			b, err := tc.marshal()
			if err != nil {
				t.Fatal(err)
			}
			s := string(b)
			idx := func(sub string) int {
				i := strings.Index(s, sub)
				if i < 0 {
					t.Fatalf("%q missing from %s", sub, s)
				}
				return i
			}
			m, d, v, n := idx("<mRID>M1</mRID>"), idx("<description>D1</description>"), idx("<version>3</version>"), idx(tc.next)
			if !(m < d && d < v && v < n) {
				t.Errorf("want mRID < description < version < %s, got %d %d %d %d in %s", tc.next, m, d, v, n, s)
			}
		})
	}
}

func TestIdentifiedObjectCopyIndependent(t *testing.T) {
	for _, tc := range identifiedCases() {
		t.Run(tc.name, func(t *testing.T) {
			orig, cp := tc.copyVer()
			if orig == nil || cp == nil || orig == cp {
				t.Fatalf("Version pointer not independently cloned: orig=%p copy=%p", orig, cp)
			}
			*cp = 99
			if *orig != 3 {
				t.Errorf("mutating the copy changed the original Version to %d", *orig)
			}
		})
	}
}

// A zero mRID must still serialize: it is minOccurs=1 in IdentifiedObject.
func TestIdentifiedObjectZeroMRIDSerializes(t *testing.T) {
	b, err := xml.Marshal(sep2.ResponseSet{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "<mRID></mRID>") {
		t.Errorf("zero mRID dropped: %s", b)
	}
}
