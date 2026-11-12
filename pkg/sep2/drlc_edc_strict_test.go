package sep2_test

import (
	"encoding/xml"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

func decodeEDC(inner string) (sep2.EndDeviceControl, error) {
	var e sep2.EndDeviceControl
	err := xml.Unmarshal([]byte(`<EndDeviceControl xmlns="urn:ieee:std:2030.5:ns">`+inner+`</EndDeviceControl>`), &e)
	return e, err
}

// TestEndDeviceControlRefusesMalformedValueTypes: a required child that is
// missing, empty, repeated, unknown or out of order is refused rather than
// read as zero.
func TestEndDeviceControlRefusesMalformedValueTypes(t *testing.T) {
	cases := map[string]string{
		"DutyCycle empty":           `<DutyCycle/>`,
		"DutyCycle empty child":     `<DutyCycle><normalValue></normalValue></DutyCycle>`,
		"DutyCycle repeated":        `<DutyCycle><normalValue>90</normalValue><normalValue>0</normalValue></DutyCycle>`,
		"DutyCycle unknown":         `<DutyCycle><normalValue>90</normalValue><extra>1</extra></DutyCycle>`,
		"DutyCycle nested":          `<DutyCycle><normalValue><x/></normalValue></DutyCycle>`,
		"ALR empty":                 `<ApplianceLoadReduction/>`,
		"ALR empty child":           `<ApplianceLoadReduction><type/></ApplianceLoadReduction>`,
		"ALR unknown":               `<ApplianceLoadReduction><kind>1</kind></ApplianceLoadReduction>`,
		"TargetReduction empty":     `<TargetReduction/>`,
		"TargetReduction no value":  `<TargetReduction><type>0</type></TargetReduction>`,
		"TargetReduction no type":   `<TargetReduction><value>5</value></TargetReduction>`,
		"TargetReduction reordered": `<TargetReduction><value>5</value><type>0</type></TargetReduction>`,
		"TargetReduction repeated":  `<TargetReduction><type>0</type><type>1</type><value>5</value></TargetReduction>`,
	}
	for name, inner := range cases {
		t.Run(name, func(t *testing.T) {
			if e, err := decodeEDC(inner); err == nil {
				t.Errorf("decoded without error: %+v %+v %+v", e.DutyCycle, e.ApplianceLoadReduction, e.TargetReduction)
			}
		})
	}
}

// TestEndDeviceControlReservedValuesRefused: values sep.xsd and IEEE
// 2030.5-2018 mark reserved or out of range are refused; the last defined
// value of each still decodes.
func TestEndDeviceControlReservedValuesRefused(t *testing.T) {
	bad := map[string]string{
		"ALR type 2":      `<ApplianceLoadReduction><type>2</type></ApplianceLoadReduction>`,
		"UnitType 13":     `<TargetReduction><type>13</type><value>1</value></TargetReduction>`,
		"DutyCycle 101":   `<DutyCycle><normalValue>101</normalValue></DutyCycle>`,
		"DutyCycle 256":   `<DutyCycle><normalValue>256</normalValue></DutyCycle>`,
		"UnitType 256":    `<TargetReduction><type>256</type><value>1</value></TargetReduction>`,
		"TR value 65536":  `<TargetReduction><type>0</type><value>65536</value></TargetReduction>`,
		"ALR type -1":     `<ApplianceLoadReduction><type>-1</type></ApplianceLoadReduction>`,
		"DutyCycle 1.5":   `<DutyCycle><normalValue>1.5</normalValue></DutyCycle>`,
		"UnitType hex 0x": `<TargetReduction><type>0x1</type><value>1</value></TargetReduction>`,
	}
	for name, inner := range bad {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeEDC(inner); err == nil {
				t.Errorf("decoded without error")
			}
		})
	}
	e, err := decodeEDC(`<ApplianceLoadReduction><type>1</type></ApplianceLoadReduction>` +
		`<DutyCycle><normalValue>100</normalValue></DutyCycle>` +
		`<TargetReduction><type>12</type><value>65535</value></TargetReduction>`)
	if err != nil {
		t.Fatalf("boundary values refused: %v", err)
	}
	if e.ApplianceLoadReduction.Type != 1 || e.DutyCycle.NormalValue != 100 ||
		e.TargetReduction.Type != 12 || e.TargetReduction.Value != 65535 {
		t.Errorf("boundary values decoded wrong: %+v %+v %+v", e.ApplianceLoadReduction, e.DutyCycle, e.TargetReduction)
	}
}

// TestEndDeviceControlBooleansAreXSDBoolean: only true, false, 1 and 0 are
// accepted, and a present but empty element is refused.
func TestEndDeviceControlBooleansAreXSDBoolean(t *testing.T) {
	for _, el := range []string{"drProgramMandatory", "loadShiftForward"} {
		for _, bad := range []string{"", "T", "TRUE", "True", "yes", "f", "2"} {
			t.Run(el+" "+bad, func(t *testing.T) {
				if e, err := decodeEDC("<" + el + ">" + bad + "</" + el + ">"); err == nil {
					t.Errorf("%q decoded: %v %v", bad, e.DRProgramMandatory, e.LoadShiftForward)
				}
			})
		}
		if _, err := decodeEDC("<" + el + "/>"); err == nil {
			t.Errorf("self-closed <%s/> decoded", el)
		}
	}
	for text, want := range map[string]bool{"true": true, "1": true, "false": false, "0": false} {
		e, err := decodeEDC("<drProgramMandatory>" + text + "</drProgramMandatory><loadShiftForward>" + text + "</loadShiftForward>")
		if err != nil {
			t.Fatalf("%q refused: %v", text, err)
		}
		if bool(e.DRProgramMandatory) != want || bool(e.LoadShiftForward) != want {
			t.Errorf("%q read as %v %v, want %v", text, e.DRProgramMandatory, e.LoadShiftForward, want)
		}
	}
}
