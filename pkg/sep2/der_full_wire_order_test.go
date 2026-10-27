package sep2_test

import (
	"encoding/xml"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// The ordered lists below are the single definition of each type's child
// element sequence, taken from the sep.xsd DERCapability and DERStatus
// sequences. connectStatus is the 2023 element and is placed where the struct
// carries it, before genConnectStatus.
var (
	derCapabilityWireSequence = []string{
		"modesSupported", "rtgAbnormalCategory", "rtgMaxA", "rtgMaxAh",
		"rtgMaxChargeRateVA", "rtgMaxChargeRateW", "rtgMaxDischargeRateVA",
		"rtgMaxDischargeRateW", "rtgMaxV", "rtgMaxVA", "rtgMaxVar",
		"rtgMaxVarNeg", "rtgMaxW", "rtgMaxWh", "rtgMinPFOverExcited",
		"rtgMinPFUnderExcited", "rtgMinV", "rtgNormalCategory",
		"rtgOverExcitedPF", "rtgOverExcitedW", "rtgReactiveSusceptance",
		"rtgUnderExcitedPF", "rtgUnderExcitedW", "rtgVNom", "type",
	}
	derStatusWireSequence = []string{
		"alarmStatus", "connectStatus", "genConnectStatus", "inverterStatus",
		"localControlModeStatus", "manufacturerStatus", "operationalModeStatus",
		"readingTime", "stateOfChargeStatus", "storageModeStatus",
		"storConnectStatus",
	}
)

// populateAll sets every pointer field of *v to a non-nil zero value and every
// non-pointer int64 field to 1, so each omitempty element is emitted.
func populateAll(v reflect.Value) {
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		switch {
		case f.Kind() == reflect.Pointer:
			f.Set(reflect.New(f.Type().Elem()))
		case f.Kind() == reflect.Int64 && f.CanSet():
			f.SetInt(1)
		}
	}
}

// childElements returns the names of the root element's direct children in
// document order.
func childElements(t *testing.T, data []byte) []string {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	var names []string
	depth := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return names
		}
		if err != nil {
			t.Fatalf("token: %v", err)
		}
		switch e := tok.(type) {
		case xml.StartElement:
			if depth == 1 {
				names = append(names, e.Name.Local)
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
}

func TestDERCapabilityFullWireOrder(t *testing.T) {
	var c sep2.DERCapability
	populateAll(reflect.ValueOf(&c).Elem())
	data, err := xml.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := childElements(t, data); !slices.Equal(got, derCapabilityWireSequence) {
		t.Errorf("DERCapability child order\n got %v\nwant %v", got, derCapabilityWireSequence)
	}
}

func TestDERStatusFullWireOrder(t *testing.T) {
	var s sep2.DERStatus
	populateAll(reflect.ValueOf(&s).Elem())
	data, err := xml.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := childElements(t, data); !slices.Equal(got, derStatusWireSequence) {
		t.Errorf("DERStatus child order\n got %v\nwant %v", got, derStatusWireSequence)
	}
}
