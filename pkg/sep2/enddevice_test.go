package sep2_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

func TestEndDeviceMarshalXML(t *testing.T) {
	enabled := true
	dev := sep2.EndDevice{
		SubscribableResource: sep2.SubscribableResource{
			Resource: sep2.Resource{Href: "/edev/1"},
		},
		ChangedTime:      1604963587,
		Enabled:          &enabled,
		SFDI:             "167261211391",
		LFDI:             "3E4F45AB31EDFE5B67E343E5E4562E31984E23E5",
		RegistrationLink: &sep2.Link{Href: "/edev/1/rg"},
	}

	data, err := xml.Marshal(&dev)
	if err != nil {
		t.Fatal(err)
	}
	xmlStr := string(data)

	if !strings.Contains(xmlStr, "urn:ieee:std:2030.5:ns") {
		t.Error("missing namespace")
	}
	if !strings.Contains(xmlStr, "<sFDI>167261211391</sFDI>") {
		t.Error("missing sFDI element")
	}
	if !strings.Contains(xmlStr, "<lFDI>3E4F45AB31EDFE5B67E343E5E4562E31984E23E5</lFDI>") {
		t.Error("missing lFDI element")
	}
	if !strings.Contains(xmlStr, "<enabled>true</enabled>") {
		t.Error("missing enabled element")
	}
}

func TestEndDeviceRoundTrip(t *testing.T) {
	enabled := true
	original := sep2.EndDevice{
		SubscribableResource: sep2.SubscribableResource{
			Resource: sep2.Resource{Href: "/edev/1"},
		},
		ChangedTime: 1604963587,
		Enabled:     &enabled,
		SFDI:        "167261211391",
		LFDI:        "3E4F45AB31EDFE5B67E343E5E4562E31984E23E5",
	}

	data, err := xml.Marshal(&original)
	if err != nil {
		t.Fatal(err)
	}

	var parsed sep2.EndDevice
	if err := xml.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed.SFDI != original.SFDI {
		t.Errorf("SFDI = %q, want %q", parsed.SFDI, original.SFDI)
	}
	if parsed.Enabled == nil || *parsed.Enabled != true {
		t.Error("Enabled not preserved")
	}
}

func TestEndDeviceListMarshalXML(t *testing.T) {
	list := sep2.EndDeviceList{
		ListResource: sep2.ListResource{
			SubscribableResource: sep2.SubscribableResource{
				Resource: sep2.Resource{Href: "/edev"},
			},
			All:     2,
			Results: 2,
		},
		EndDevice: []sep2.EndDevice{
			{SubscribableResource: sep2.SubscribableResource{Resource: sep2.Resource{Href: "/edev/1"}}, SFDI: "111"},
			{SubscribableResource: sep2.SubscribableResource{Resource: sep2.Resource{Href: "/edev/2"}}, SFDI: "222"},
		},
	}

	data, err := xml.Marshal(&list)
	if err != nil {
		t.Fatal(err)
	}
	xmlStr := string(data)

	if !strings.Contains(xmlStr, "EndDeviceList") {
		t.Error("missing EndDeviceList element")
	}
	if !strings.Contains(xmlStr, `all="2"`) {
		t.Error("missing all attribute")
	}
	if !strings.Contains(xmlStr, `results="2"`) {
		t.Error("missing results attribute")
	}
}

func TestRegistrationMarshalXML(t *testing.T) {
	reg := sep2.Registration{
		Resource:           sep2.Resource{Href: "/edev/1/rg"},
		DateTimeRegistered: 1604963587,
		PIN:                12345,
	}

	data, err := xml.Marshal(&reg)
	if err != nil {
		t.Fatal(err)
	}
	xmlStr := string(data)

	if !strings.Contains(xmlStr, "Registration") {
		t.Error("missing Registration element")
	}
	if !strings.Contains(xmlStr, "<pIN>12345</pIN>") {
		t.Error("missing pIN element")
	}
}

func TestEndDeviceCopy(t *testing.T) {
	enabled := true
	original := sep2.EndDevice{
		Enabled:          &enabled,
		SFDI:             "123",
		RegistrationLink: &sep2.Link{Href: "/rg"},
	}

	copied := original.Copy()

	// Mutate copy
	*copied.Enabled = false
	copied.SFDI = "999"
	copied.RegistrationLink.Href = "/changed"

	// Original should be unchanged
	if *original.Enabled != true {
		t.Error("original Enabled was mutated")
	}
	if original.SFDI != "123" {
		t.Error("original SFDI was mutated")
	}
	if original.RegistrationLink.Href != "/rg" {
		t.Error("original RegistrationLink was mutated")
	}
}

// SubscriptionListLink (GRIDAPPSD/ieee-2030_5-server-go#180) must
// round-trip when present and be omitted when absent (omitempty).
// Backward XML compatibility for servers that don't advertise
// subscription support.
func TestEndDeviceSubscriptionListLink_RoundTrip(t *testing.T) {
	original := sep2.EndDevice{
		SubscribableResource: sep2.SubscribableResource{
			Resource: sep2.Resource{Href: "/edev/1"},
		},
		ChangedTime:          1604963587,
		SFDI:                 "111",
		SubscriptionListLink: &sep2.ListLink{Href: "/edev/1/sub", All: 0},
	}

	data, err := xml.Marshal(&original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	xmlStr := string(data)
	if !strings.Contains(xmlStr, `<SubscriptionListLink href="/edev/1/sub"`) {
		t.Errorf("missing SubscriptionListLink href: %s", xmlStr)
	}

	var parsed sep2.EndDevice
	if err := xml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.SubscriptionListLink == nil {
		t.Fatal("SubscriptionListLink lost on round-trip")
	}
	if parsed.SubscriptionListLink.Href != "/edev/1/sub" {
		t.Errorf("Href = %q, want /edev/1/sub", parsed.SubscriptionListLink.Href)
	}
}

func TestEndDeviceSubscriptionListLink_OmittedWhenNil(t *testing.T) {
	original := sep2.EndDevice{
		SubscribableResource: sep2.SubscribableResource{
			Resource: sep2.Resource{Href: "/edev/1"},
		},
		SFDI: "111",
		// SubscriptionListLink intentionally nil
	}
	data, err := xml.Marshal(&original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "SubscriptionListLink") {
		t.Errorf("nil SubscriptionListLink leaked into XML: %s", string(data))
	}
}

// TestEndDeviceFlowReservationLinksOmittedByteIdentical proves that leaving
// both new flow reservation links nil (GRIDAPPSD/ieee-2030_5-core-go#177)
// reproduces byte-for-byte what the pre-#177 struct produced for the same
// populated fields: an omitempty insertion emits nothing on its own.
// Sequence position is proven separately, by TestEndDeviceWireOrder, not by
// this test. The golden string below was captured by marshalling this same
// EndDevice against the struct as it existed at origin/main before #177
// (base e7ad354).
func TestEndDeviceFlowReservationLinksOmittedByteIdentical(t *testing.T) {
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
		LogEventListLink:               &sep2.ListLink{Href: "/edev/1/lel"},
		FunctionSetAssignmentsListLink: &sep2.ListLink{Href: "/edev/1/fsa"},
		RegistrationLink:               &sep2.Link{Href: "/edev/1/rg"},
		SubscriptionListLink:           &sep2.ListLink{Href: "/edev/1/sub"},
		// FlowReservationRequestListLink and FlowReservationResponseListLink
		// intentionally left nil.
	}

	const golden = `<EndDevice xmlns="urn:ieee:std:2030.5:ns" href="/edev/1"><DERListLink href="/edev/1/der"></DERListLink><lFDI>3E4F45AB31EDFE5B67E343E5E4562E31984E23E5</lFDI><LogEventListLink href="/edev/1/lel"></LogEventListLink><sFDI>167261211391</sFDI><changedTime>1604963587</changedTime><enabled>true</enabled><FunctionSetAssignmentsListLink href="/edev/1/fsa"></FunctionSetAssignmentsListLink><RegistrationLink href="/edev/1/rg"></RegistrationLink><SubscriptionListLink href="/edev/1/sub"></SubscriptionListLink></EndDevice>`

	data, err := xml.Marshal(&dev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) != golden {
		t.Errorf("marshal with both flow reservation links nil is not byte-identical to pre-#177 output:\ngot:  %s\nwant: %s", data, golden)
	}
}

// Copy() must deep-copy the SubscriptionListLink field
// (GRIDAPPSD/ieee-2030_5-server-go#180) so downstream mutation doesn't
// leak into the original (matches the pattern for the other *Link /
// *ListLink fields).
func TestEndDeviceCopy_SubscriptionListLink(t *testing.T) {
	original := sep2.EndDevice{
		SFDI:                 "123",
		SubscriptionListLink: &sep2.ListLink{Href: "/sub", All: 3},
	}

	copied := original.Copy()
	copied.SubscriptionListLink.Href = "/sub-changed"
	copied.SubscriptionListLink.All = 99

	if original.SubscriptionListLink.Href != "/sub" {
		t.Errorf("original SubscriptionListLink.Href mutated: %q", original.SubscriptionListLink.Href)
	}
	if original.SubscriptionListLink.All != 3 {
		t.Errorf("original SubscriptionListLink.All mutated: %d", original.SubscriptionListLink.All)
	}
}

// Copy() must deep-copy the FlowReservationRequestListLink and
// FlowReservationResponseListLink fields (GRIDAPPSD/ieee-2030_5-core-go#177)
// so downstream mutation doesn't leak into the original (matches the
// pattern for the other *Link / *ListLink fields).
func TestEndDeviceCopy_FlowReservationLinks(t *testing.T) {
	original := sep2.EndDevice{
		SFDI:                            "123",
		FlowReservationRequestListLink:  &sep2.ListLink{Href: "/frq", All: 1},
		FlowReservationResponseListLink: &sep2.ListLink{Href: "/frs", All: 2},
	}

	copied := original.Copy()
	if copied.FlowReservationRequestListLink == original.FlowReservationRequestListLink {
		t.Error("FlowReservationRequestListLink not deep-copied: same pointer as original")
	}
	if copied.FlowReservationResponseListLink == original.FlowReservationResponseListLink {
		t.Error("FlowReservationResponseListLink not deep-copied: same pointer as original")
	}

	copied.FlowReservationRequestListLink.Href = "/frq-changed"
	copied.FlowReservationRequestListLink.All = 99
	copied.FlowReservationResponseListLink.Href = "/frs-changed"
	copied.FlowReservationResponseListLink.All = 98

	if original.FlowReservationRequestListLink.Href != "/frq" {
		t.Errorf("original FlowReservationRequestListLink.Href mutated: %q", original.FlowReservationRequestListLink.Href)
	}
	if original.FlowReservationRequestListLink.All != 1 {
		t.Errorf("original FlowReservationRequestListLink.All mutated: %d", original.FlowReservationRequestListLink.All)
	}
	if original.FlowReservationResponseListLink.Href != "/frs" {
		t.Errorf("original FlowReservationResponseListLink.Href mutated: %q", original.FlowReservationResponseListLink.Href)
	}
	if original.FlowReservationResponseListLink.All != 2 {
		t.Errorf("original FlowReservationResponseListLink.All mutated: %d", original.FlowReservationResponseListLink.All)
	}
}
