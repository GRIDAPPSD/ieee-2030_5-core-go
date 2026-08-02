package sep2_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// TestRegistrationWireOrder asserts Registration element order against
// sep.xsd's complexType "Registration", which extends Resource and declares
// the xsd:sequence: dateTimeRegistered (minOccurs=1), pIN (minOccurs=1).
// pollRate is an xsd:attribute on the same complexType, not a sequence
// element, so it must marshal as an attribute of the root element rather
// than as a child.
//
// A strict client (the EPRI oeg_client generated from this same schema)
// validates the sequence and rejects out-of-order XML, so the order is a
// wire-level invariant, not a cosmetic one.
func TestRegistrationWireOrder(t *testing.T) {
	reg := sep2.Registration{
		Resource:           sep2.Resource{Href: "/edev/1/rg"},
		DateTimeRegistered: 1604963587,
		PIN:                111115,
		PollRate:           900,
	}

	data, err := xml.Marshal(&reg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	xmlStr := string(data)

	assertOrder(t, xmlStr, []string{
		"<dateTimeRegistered>",
		"<pIN>",
	})

	// pollRate is an attribute on the Registration element, not a child
	// element. Assert the attribute form explicitly: a Go tag that lost
	// its ",attr" would still round-trip through encoding/xml but would
	// emit <pollRate>900</pollRate>, which fails a strict schema parse.
	if !strings.Contains(xmlStr, `pollRate="900"`) {
		t.Errorf("pollRate did not marshal as an attribute; got:\n%s", xmlStr)
	}
	if strings.Contains(xmlStr, "<pollRate>") {
		t.Errorf("pollRate marshalled as a child element, but sep.xsd declares it an attribute; got:\n%s", xmlStr)
	}

	// The two sequence elements are required (minOccurs=1), so they must
	// be present even at their zero values. Guard against a stray
	// omitempty being added to either.
	zero := sep2.Registration{}
	zeroData, err := xml.Marshal(&zero)
	if err != nil {
		t.Fatalf("marshal zero value: %v", err)
	}
	for _, elem := range []string{"<dateTimeRegistered>", "<pIN>"} {
		if !strings.Contains(string(zeroData), elem) {
			t.Errorf("required element %s is absent at zero value (omitempty added?); got:\n%s", elem, zeroData)
		}
	}
}
