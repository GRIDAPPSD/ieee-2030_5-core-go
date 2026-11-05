package sep2_test

import (
	"encoding/xml"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// TestEndDeviceWireOrder asserts EndDevice element order against sep.xsd's
// AbstractDevice + EndDevice combined xsd:sequence (AbstractDevice's own
// sequence elements come first, per xsd:extension composition, then
// EndDevice's own sequence elements): ConfigurationLink, DERListLink,
// deviceCategory, DeviceInformationLink, DeviceStatusLink, FileStatusLink,
// IPInterfaceListLink, lFDI, LoadShedAvailabilityListLink, LogEventListLink,
// PowerStatusLink, sFDI (AbstractDevice) THEN changedTime, enabled,
// FlowReservationRequestListLink, FlowReservationResponseListLink,
// FunctionSetAssignmentsListLink, postRate, RegistrationLink,
// SubscriptionListLink (EndDevice's own sequence).
//
// The non-link elements deviceCategory and postRate are not modeled on the Go
// struct and are not asserted.
func TestEndDeviceWireOrder(t *testing.T) {
	enabled := true
	dev := sep2.EndDevice{
		SubscribableResource: sep2.SubscribableResource{
			Resource: sep2.Resource{Href: "/edev/1"},
		},
		ChangedTime:                     1604963587,
		Enabled:                         &enabled,
		LFDI:                            "3E4F45AB31EDFE5B67E343E5E4562E31984E23E5",
		SFDI:                            "167261211391",
		ConfigurationLink:               &sep2.Link{Href: "/edev/1/cfg"},
		DERListLink:                     &sep2.ListLink{Href: "/edev/1/der"},
		DeviceInformationLink:           &sep2.Link{Href: "/edev/1/di"},
		DeviceStatusLink:                &sep2.Link{Href: "/edev/1/dstat"},
		FileStatusLink:                  &sep2.Link{Href: "/edev/1/fs"},
		IPInterfaceListLink:             &sep2.ListLink{Href: "/edev/1/ns"},
		LoadShedAvailabilityListLink:    &sep2.ListLink{Href: "/edev/1/lsl"},
		PowerStatusLink:                 &sep2.Link{Href: "/edev/1/ps"},
		LogEventListLink:                &sep2.ListLink{Href: "/edev/1/lel"},
		FlowReservationRequestListLink:  &sep2.ListLink{Href: "/edev/1/frq"},
		FlowReservationResponseListLink: &sep2.ListLink{Href: "/edev/1/frs"},
		FunctionSetAssignmentsListLink:  &sep2.ListLink{Href: "/edev/1/fsa"},
		RegistrationLink:                &sep2.Link{Href: "/edev/1/rg"},
		SubscriptionListLink:            &sep2.ListLink{Href: "/edev/1/sub"},
	}

	data, err := xml.Marshal(&dev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	xmlStr := string(data)

	// AbstractDevice sequence must precede EndDevice's own sequence.
	assertOrder(t, xmlStr, []string{
		"<ConfigurationLink",
		"<DERListLink",
		"<DeviceInformationLink",
		"<DeviceStatusLink",
		"<FileStatusLink",
		"<IPInterfaceListLink",
		"<lFDI>",
		"<LoadShedAvailabilityListLink",
		"<LogEventListLink",
		"<PowerStatusLink",
		"<sFDI>",
		"<changedTime>",
		"<enabled>",
		"<FlowReservationRequestListLink",
		"<FlowReservationResponseListLink",
		"<FunctionSetAssignmentsListLink",
		"<RegistrationLink",
		"<SubscriptionListLink",
	})
}
