package sep2_test

import (
	"encoding/xml"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// TestFunctionSetAssignmentsWireOrder asserts FunctionSetAssignments order
// against sep.xsd's FunctionSetAssignmentsBase + FunctionSetAssignments
// combined sequence: DemandResponseProgramListLink, DERProgramListLink,
// UsagePointListLink (FunctionSetAssignmentsBase, alpha order) THEN mRID,
// description (FunctionSetAssignments' own sequence). The regression this
// guards: mRID/description were previously emitted first, ahead of the
// base's Link fields.
func TestFunctionSetAssignmentsWireOrder(t *testing.T) {
	fsa := sep2.FunctionSetAssignments{
		MRID:                          "FSA001",
		Description:                   "test fsa",
		DERProgramListLink:            &sep2.ListLink{Href: "/derp"},
		UsagePointListLink:            &sep2.ListLink{Href: "/upt"},
		DemandResponseProgramListLink: &sep2.ListLink{Href: "/drp"},
	}

	data, err := xml.Marshal(&fsa)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	xmlStr := string(data)

	assertOrder(t, xmlStr, []string{
		"<DemandResponseProgramListLink",
		"<DERProgramListLink",
		"<UsagePointListLink",
		"<mRID>",
		"<description>",
	})
}
