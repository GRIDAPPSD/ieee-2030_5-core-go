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
// Only fields that currently exist on the Go struct are asserted; fields
// absent from the Go type (ConfigurationLink, deviceCategory, etc.) are out
// of scope for this reorder and are reported as findings, not added here.
func TestEndDeviceWireOrder(t *testing.T) {
	enabled := true
	dev := sep2.EndDevice{
		SubscribableResource: sep2.SubscribableResource{
			Resource: sep2.Resource{Href: "/edev/1"},
		},
		ChangedTime:                    1604963587,
		Enabled:                        &enabled,
		LFDI:                           "3E4F45AB31EDFE5B67E343E5E4562E31984E23E5",
		SFDI:                           "167261211391",
		DERListLink:                    &sep2.ListLink{Href: "/edev/1/der"},
		LogEventListLink:               &sep2.ListLink{Href: "/edev/1/log"},
		FunctionSetAssignmentsListLink: &sep2.ListLink{Href: "/edev/1/fsa"},
		RegistrationLink:               &sep2.Link{Href: "/edev/1/rg"},
		SubscriptionListLink:           &sep2.ListLink{Href: "/edev/1/sub"},
	}

	data, err := xml.Marshal(&dev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	xmlStr := string(data)

	// AbstractDevice sequence (DERListLink, lFDI, LogEventListLink, sFDI)
	// must precede EndDevice's own sequence (changedTime, enabled,
	// FunctionSetAssignmentsListLink, RegistrationLink,
	// SubscriptionListLink).
	assertOrder(t, xmlStr, []string{
		"<DERListLink",
		"<lFDI>",
		"<LogEventListLink",
		"<sFDI>",
		"<changedTime>",
		"<enabled>",
		"<FunctionSetAssignmentsListLink",
		"<RegistrationLink",
		"<SubscriptionListLink",
	})
}
