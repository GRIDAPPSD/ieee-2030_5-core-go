package sep2_test

import (
	"encoding/xml"
	"strconv"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/internal/xsdgate"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

func versionPtr(v uint16) *uint16 { return &v }

func TestDERProgramVersionRoundTrip(t *testing.T) {
	with := sep2.DERProgram{
		MRID:                  "PROG001",
		Description:           "Solar",
		Version:               versionPtr(7),
		DefaultDERControlLink: &sep2.Link{Href: "/derp/1/dderc"},
		Primacy:               10,
	}
	data, err := xml.Marshal(&with)
	if err != nil {
		t.Fatal(err)
	}
	assertOrder(t, string(data), []string{
		"<mRID>", "<description>", "<version>7</version>",
		"<DefaultDERControlLink", "<primacy>",
	})
	var parsed sep2.DERProgram
	if err := xml.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Version == nil || *parsed.Version != 7 {
		t.Fatalf("Version = %v, want 7", parsed.Version)
	}

	without := sep2.DERProgram{MRID: "PROG001", Primacy: 10}
	data, err = xml.Marshal(&without)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "version") {
		t.Errorf("absent version emitted: %s", data)
	}
	parsed = sep2.DERProgram{}
	if err := xml.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Version != nil {
		t.Errorf("Version = %d, want nil", *parsed.Version)
	}
}

func TestDefaultDERControlVersionRoundTrip(t *testing.T) {
	connected := true
	grad := uint16(50)
	with := sep2.DefaultDERControl{
		MRID:           "DDC001",
		Version:        versionPtr(3),
		DERControlBase: &sep2.DERControlBase{OpModConnect: &connected},
		SetGradW:       &grad,
	}
	data, err := xml.Marshal(&with)
	if err != nil {
		t.Fatal(err)
	}
	assertOrder(t, string(data), []string{
		"<mRID>", "<version>3</version>", "<DERControlBase>", "<setGradW>",
	})
	var parsed sep2.DefaultDERControl
	if err := xml.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Version == nil || *parsed.Version != 3 {
		t.Fatalf("Version = %v, want 3", parsed.Version)
	}

	without := sep2.DefaultDERControl{MRID: "DDC001", DERControlBase: &sep2.DERControlBase{OpModConnect: &connected}}
	data, err = xml.Marshal(&without)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "version") {
		t.Errorf("absent version emitted: %s", data)
	}
	parsed = sep2.DefaultDERControl{}
	if err := xml.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Version != nil {
		t.Errorf("Version = %d, want nil", *parsed.Version)
	}
}

func TestDERProgramVersionCopyIsDeep(t *testing.T) {
	orig := sep2.DERProgram{Version: versionPtr(7)}
	c := orig.Copy()
	*c.Version = 99
	if *orig.Version != 7 {
		t.Errorf("Copy aliased Version: original now %d", *orig.Version)
	}
	if got := (sep2.DERProgram{}).Copy().Version; got != nil {
		t.Errorf("Copy of nil Version = %v, want nil", got)
	}
}

func TestDefaultDERControlVersionCopyIsDeep(t *testing.T) {
	orig := sep2.DefaultDERControl{Version: versionPtr(3)}
	c := orig.Copy()
	*c.Version = 99
	if *orig.Version != 3 {
		t.Errorf("Copy aliased Version: original now %d", *orig.Version)
	}
	if got := (sep2.DefaultDERControl{}).Copy().Version; got != nil {
		t.Errorf("Copy of nil Version = %v, want nil", got)
	}
}

func TestDERVersionSchemaValid(t *testing.T) {
	connected := true
	for _, v := range []*uint16{nil, versionPtr(0), versionPtr(7), versionPtr(65535)} {
		xsdgate.AssertValid(t, "DERProgram", &sep2.DERProgram{
			SubscribableResource: sep2.SubscribableResource{Resource: sep2.Resource{Href: "/derp/1"}},
			MRID:                 "00000000000000000000000000000001",
			Description:          "Solar",
			Version:              v,
		})
		xsdgate.AssertValid(t, "DefaultDERControl", &sep2.DefaultDERControl{
			SubscribableResource: sep2.SubscribableResource{Resource: sep2.Resource{Href: "/derp/1/dderc"}},
			MRID:                 "00000000000000000000000000000002",
			Version:              v,
			DERControlBase:       &sep2.DERControlBase{OpModConnect: &connected},
		})
	}
}

func TestDERVersionBounds(t *testing.T) {
	for _, tc := range []struct {
		name string
		v    uint16
	}{{"zero", 0}, {"max", 65535}} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := xml.Marshal(&sep2.DERProgram{Version: versionPtr(tc.v)})
			if err != nil {
				t.Fatal(err)
			}
			want := "<version>" + strconv.Itoa(int(tc.v)) + "</version>"
			if !strings.Contains(string(data), want) {
				t.Fatalf("missing %s in %s", want, data)
			}
			var parsed sep2.DERProgram
			if err := xml.Unmarshal(data, &parsed); err != nil {
				t.Fatal(err)
			}
			if parsed.Version == nil || *parsed.Version != tc.v {
				t.Errorf("Version = %v, want %d", parsed.Version, tc.v)
			}
		})
	}

	over := `<DERProgram xmlns="urn:ieee:std:2030.5:ns"><version>65536</version></DERProgram>`
	var parsed sep2.DERProgram
	if err := xml.Unmarshal([]byte(over), &parsed); err == nil {
		t.Errorf("version 65536 parsed without error: %v", parsed.Version)
	}
}
