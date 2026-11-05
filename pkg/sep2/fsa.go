package sep2

import "encoding/xml"

// FunctionSetAssignments maps function sets to a device.
// Spec reference: section 8.6
//
// Field order matches the FunctionSetAssignmentsBase + FunctionSetAssignments
// combined xsd:sequence: FunctionSetAssignmentsBase's own Link fields
// (CustomerAccountListLink, DemandResponseProgramListLink, DERProgramListLink,
// FileListLink, MessagingProgramListLink, PrepaymentListLink,
// ResponseSetListLink, TariffProfileListLink, TimeLink, UsagePointListLink)
// come FIRST, THEN FunctionSetAssignments' own
// sequence (mRID, description; version omitted, not yet modeled).
type FunctionSetAssignments struct {
	XMLName xml.Name `xml:"urn:ieee:std:2030.5:ns FunctionSetAssignments"`
	Resource

	CustomerAccountListLink       *ListLink `xml:"CustomerAccountListLink,omitempty"`
	DemandResponseProgramListLink *ListLink `xml:"DemandResponseProgramListLink,omitempty"`
	DERProgramListLink            *ListLink `xml:"DERProgramListLink,omitempty"`
	FileListLink                  *ListLink `xml:"FileListLink,omitempty"`
	MessagingProgramListLink      *ListLink `xml:"MessagingProgramListLink,omitempty"`
	PrepaymentListLink            *ListLink `xml:"PrepaymentListLink,omitempty"`
	ResponseSetListLink           *ListLink `xml:"ResponseSetListLink,omitempty"`
	TariffProfileListLink         *ListLink `xml:"TariffProfileListLink,omitempty"`
	TimeLink                      *Link     `xml:"TimeLink,omitempty"`
	UsagePointListLink            *ListLink `xml:"UsagePointListLink,omitempty"`

	MRID        string `xml:"mRID,omitempty"`
	Description string `xml:"description,omitempty"`
}

// Copy returns an independent copy.
func (f FunctionSetAssignments) Copy() FunctionSetAssignments {
	c := f
	c.CustomerAccountListLink = clonePtr(f.CustomerAccountListLink)
	c.DemandResponseProgramListLink = clonePtr(f.DemandResponseProgramListLink)
	c.DERProgramListLink = clonePtr(f.DERProgramListLink)
	c.FileListLink = clonePtr(f.FileListLink)
	c.MessagingProgramListLink = clonePtr(f.MessagingProgramListLink)
	c.PrepaymentListLink = clonePtr(f.PrepaymentListLink)
	c.ResponseSetListLink = clonePtr(f.ResponseSetListLink)
	c.TariffProfileListLink = clonePtr(f.TariffProfileListLink)
	c.TimeLink = clonePtr(f.TimeLink)
	c.UsagePointListLink = clonePtr(f.UsagePointListLink)
	return c
}

// FunctionSetAssignmentsList is a list of FunctionSetAssignments.
type FunctionSetAssignmentsList struct {
	XMLName xml.Name `xml:"urn:ieee:std:2030.5:ns FunctionSetAssignmentsList"`
	ListResource
	FunctionSetAssignments []FunctionSetAssignments `xml:"FunctionSetAssignments,omitempty"`
}
