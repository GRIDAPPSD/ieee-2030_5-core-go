package sep2_test

import (
	"encoding/xml"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// looseDRLCValues are Offset, SetPoint and overrideDuration bodies that
// must be refused wherever the value is decoded.
var looseDRLCValues = map[string]string{
	"empty overrideDuration":   `<overrideDuration/>`,
	"empty coolingOffset":      `<Offset><coolingOffset/></Offset>`,
	"empty heatingOffset":      `<Offset><heatingOffset></heatingOffset></Offset>`,
	"empty loadAdjustment":     `<Offset><loadAdjustmentPercentageOffset/></Offset>`,
	"empty coolingSetpoint":    `<SetPoint><coolingSetpoint/></SetPoint>`,
	"empty heatingSetpoint":    `<SetPoint><heatingSetpoint/></SetPoint>`,
	"repeated coolingOffset":   `<Offset><coolingOffset>1</coolingOffset><coolingOffset>2</coolingOffset></Offset>`,
	"unknown Offset child":     `<Offset><coolingOffset>1</coolingOffset><bogus>1</bogus></Offset>`,
	"foreign namespace child":  `<Offset><coolingOffset xmlns="urn:example:other">1</coolingOffset></Offset>`,
	"nested element":           `<Offset><coolingOffset><x/>1</coolingOffset></Offset>`,
	"Offset out of order":      `<Offset><heatingOffset>1</heatingOffset><coolingOffset>2</coolingOffset></Offset>`,
	"repeated coolingSetpoint": `<SetPoint><coolingSetpoint>1</coolingSetpoint><coolingSetpoint>2</coolingSetpoint></SetPoint>`,
	"unknown SetPoint child":   `<SetPoint><bogus>1</bogus></SetPoint>`,
	"SetPoint out of order":    `<SetPoint><heatingSetpoint>1</heatingSetpoint><coolingSetpoint>2</coolingSetpoint></SetPoint>`,
	"nested setpoint":          `<SetPoint><coolingSetpoint><x/>1</coolingSetpoint></SetPoint>`,
	"loadAdjustment too big":   `<Offset><loadAdjustmentPercentageOffset>10001</loadAdjustmentPercentageOffset></Offset>`,
	"coolingOffset too big":    `<Offset><coolingOffset>256</coolingOffset></Offset>`,
	"coolingSetpoint too big":  `<SetPoint><coolingSetpoint>40000</coolingSetpoint></SetPoint>`,
}

func TestDRLCValuesRefuseLooseInput(t *testing.T) {
	t.Parallel()
	for name, child := range looseDRLCValues {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dr := `<DrResponse xmlns="urn:ieee:std:2030.5:ns">` + child + `</DrResponse>`
			if _, err := sep2.DecodeResponse([]byte(dr)); err == nil {
				t.Errorf("DrResponse: DecodeResponse = nil error, want refused")
			}
			var e sep2.EndDeviceControl
			edc := `<EndDeviceControl xmlns="urn:ieee:std:2030.5:ns">` + child + `</EndDeviceControl>`
			if err := xml.Unmarshal([]byte(edc), &e); err == nil {
				t.Errorf("EndDeviceControl: Unmarshal = nil error, want refused")
			}
		})
	}
}

func TestOffsetAndSetPointAcceptAnySubsetInOrder(t *testing.T) {
	t.Parallel()
	var o sep2.Offset
	if err := xml.Unmarshal([]byte(`<Offset xmlns="urn:ieee:std:2030.5:ns"><heatingOffset>7</heatingOffset></Offset>`), &o); err != nil {
		t.Fatalf("Offset with only heatingOffset: %v", err)
	}
	if o.CoolingOffset != nil || o.HeatingOffset == nil || *o.HeatingOffset != 7 || o.LoadAdjustmentPercentageOffset != nil {
		t.Errorf("Offset = %+v, want only heatingOffset 7", o)
	}
	var s sep2.SetPoint
	if err := xml.Unmarshal([]byte(`<SetPoint xmlns="urn:ieee:std:2030.5:ns"><heatingSetpoint>-5</heatingSetpoint></SetPoint>`), &s); err != nil {
		t.Fatalf("SetPoint with only heatingSetpoint: %v", err)
	}
	if s.CoolingSetpoint != nil || s.HeatingSetpoint == nil || *s.HeatingSetpoint != -5 {
		t.Errorf("SetPoint = %+v, want only heatingSetpoint -5", s)
	}
	var empty sep2.Offset
	if err := xml.Unmarshal([]byte(`<Offset xmlns="urn:ieee:std:2030.5:ns"></Offset>`), &empty); err != nil {
		t.Errorf("empty Offset element: %v, want accepted (every child is minOccurs 0)", err)
	}
}

// TestDRLCValueFieldsAgree fails when EndDeviceControl and DrResponseDRLC
// stop carrying the same DRLC value fields in the same relative order with
// the same types and tags. DrResponse reports AppliedTargetReduction where
// EndDeviceControl carries TargetReduction.
func TestDRLCValueFieldsAgree(t *testing.T) {
	t.Parallel()
	edcOnly := map[string]bool{
		"XMLName": true, "RandomizableEvent": true, "DeviceCategory": true,
		"DRProgramMandatory": true, "LoadShiftForward": true,
	}
	rename := map[string]string{"TargetReduction": "AppliedTargetReduction"}

	type field struct{ name, typ, tag string }
	var edc, dr []field
	et := reflect.TypeOf(sep2.EndDeviceControl{})
	for i := 0; i < et.NumField(); i++ {
		f := et.Field(i)
		if edcOnly[f.Name] {
			continue
		}
		name := f.Name
		if r, ok := rename[name]; ok {
			name = r
		}
		tag := f.Tag.Get("xml")
		if f.Name == "TargetReduction" {
			tag = strings.Replace(tag, "TargetReduction", "AppliedTargetReduction", 1)
		}
		edc = append(edc, field{name, f.Type.String(), tag})
	}
	dt := reflect.TypeOf(sep2.DrResponseDRLC{})
	for i := 0; i < dt.NumField(); i++ {
		f := dt.Field(i)
		dr = append(dr, field{f.Name, f.Type.String(), f.Tag.Get("xml")})
	}
	// DrResponse puts AppliedTargetReduction second in sequence order and
	// EndDeviceControl puts TargetReduction last, so compare as sets keyed
	// by name, then the order of the fields both carry.
	byName := func(fs []field) map[string]field {
		m := map[string]field{}
		for _, f := range fs {
			m[f.name] = f
		}
		return m
	}
	em, dm := byName(edc), byName(dr)
	if len(em) != len(dm) {
		t.Errorf("EndDeviceControl has %d DRLC value fields, DrResponseDRLC has %d: %v vs %v", len(em), len(dm), edc, dr)
	}
	for n, f := range dm {
		if em[n] != f {
			t.Errorf("DRLC field %s: DrResponseDRLC %+v, EndDeviceControl %+v", n, f, em[n])
		}
	}
	order := func(fs []field) string {
		var s []string
		for _, f := range fs {
			if f.name != "AppliedTargetReduction" {
				s = append(s, f.name)
			}
		}
		return fmt.Sprint(s)
	}
	if order(edc) != order(dr) {
		t.Errorf("shared DRLC field order differs: EndDeviceControl %s, DrResponseDRLC %s", order(edc), order(dr))
	}
}
