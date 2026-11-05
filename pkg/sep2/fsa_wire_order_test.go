package sep2_test

import (
	"encoding/xml"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// TestFunctionSetAssignmentsWireOrder asserts FunctionSetAssignments order
// against sep.xsd's FunctionSetAssignmentsBase + FunctionSetAssignments
// combined sequence: the ten FunctionSetAssignmentsBase Link elements THEN
// mRID, description (FunctionSetAssignments' own sequence). The regression this
// guards: mRID/description were previously emitted first, ahead of the
// base's Link fields.
func TestFunctionSetAssignmentsWireOrder(t *testing.T) {
	fsa := sep2.FunctionSetAssignments{
		MRID:        "FSA001",
		Description: "test fsa",
		FunctionSetAssignmentsBase: sep2.FunctionSetAssignmentsBase{
			CustomerAccountListLink:       &sep2.ListLink{Href: "/ca"},
			DemandResponseProgramListLink: &sep2.ListLink{Href: "/drp"},
			DERProgramListLink:            &sep2.ListLink{Href: "/derp"},
			FileListLink:                  &sep2.ListLink{Href: "/file"},
			MessagingProgramListLink:      &sep2.ListLink{Href: "/msg"},
			PrepaymentListLink:            &sep2.ListLink{Href: "/ppy"},
			ResponseSetListLink:           &sep2.ListLink{Href: "/rsps"},
			TariffProfileListLink:         &sep2.ListLink{Href: "/tp"},
			TimeLink:                      &sep2.Link{Href: "/tm"},
			UsagePointListLink:            &sep2.ListLink{Href: "/upt"},
		},
	}

	data, err := xml.Marshal(&fsa)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	xmlStr := string(data)

	assertOrder(t, xmlStr, []string{
		"<CustomerAccountListLink",
		"<DemandResponseProgramListLink",
		"<DERProgramListLink",
		"<FileListLink",
		"<MessagingProgramListLink",
		"<PrepaymentListLink",
		"<ResponseSetListLink",
		"<TariffProfileListLink",
		"<TimeLink",
		"<UsagePointListLink",
		"<mRID>",
		"<description>",
	})
}
