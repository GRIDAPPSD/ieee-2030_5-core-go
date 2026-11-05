package sep2_test

import (
	"encoding/xml"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// TestDERLinkWireOrder pins the DER Link elements to the sep.xsd DER sequence.
func TestDERLinkWireOrder(t *testing.T) {
	d := sep2.DER{
		AssociatedDERProgramListLink: &sep2.ListLink{Href: "/a"},
		AssociatedUsagePointLink:     &sep2.Link{Href: "/b"},
		CurrentDERProgramLink:        &sep2.Link{Href: "/c"},
		DERAvailabilityLink:          &sep2.Link{Href: "/d"},
		DERCapabilityLink:            &sep2.Link{Href: "/e"},
		DERSettingsLink:              &sep2.Link{Href: "/f"},
		DERStatusLink:                &sep2.Link{Href: "/g"},
	}
	data, err := xml.Marshal(&d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	assertOrder(t, string(data), []string{
		"<AssociatedDERProgramListLink",
		"<AssociatedUsagePointLink",
		"<CurrentDERProgramLink",
		"<DERAvailabilityLink",
		"<DERCapabilityLink",
		"<DERSettingsLink",
		"<DERStatusLink",
	})
}

// TestDeviceInformationLinkWireOrder pins SupportedLocaleListLink between
// secondaryPower and swActTime.
func TestDeviceInformationLinkWireOrder(t *testing.T) {
	d := sep2.DeviceInformation{
		LFDI:                    "ABC",
		SecondaryPower:          1,
		SupportedLocaleListLink: &sep2.ListLink{Href: "/loc", All: 2},
		SwActTime:               5,
		SwVer:                   "1",
	}
	data, err := xml.Marshal(&d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	assertOrder(t, string(data), []string{
		"<lFDI>",
		"<secondaryPower>",
		"<SupportedLocaleListLink",
		"<swActTime>",
		"<swVer>",
	})
}

// TestConfigurationLinkWireOrder pins PriceResponseCfgListLink between
// PowerConfiguration and TimeConfiguration.
func TestConfigurationLinkWireOrder(t *testing.T) {
	c := sep2.Configuration{
		CurrentLocale:            "en",
		PowerConfiguration:       &sep2.PowerConfiguration{},
		PriceResponseCfgListLink: &sep2.ListLink{Href: "/prc"},
		TimeConfiguration:        &sep2.TimeConfiguration{},
		UserDeviceName:           "dev",
	}
	data, err := xml.Marshal(&c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	assertOrder(t, string(data), []string{
		"<currentLocale>",
		"<PowerConfiguration",
		"<PriceResponseCfgListLink",
		"<TimeConfiguration",
		"<userDeviceName>",
	})
}
