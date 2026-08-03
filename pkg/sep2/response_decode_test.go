package sep2_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// The Response POST body is polymorphic, and that is the whole of
// IEEECORE-067's second defect.
//
// sep.xsd declares Response (sep.xsd:502-532) plus five types that extend it:
// DERControlResponse (:440-447), FlowReservationResponseResponse (:448-454),
// PriceResponse (:494-501), TextResponse (:574-581) and DrResponse (:473-493).
// The WADL puts SIX resources at one sample path, /rsps/{id1}/rsp/{id2}:
// Response, PriceResponse, TextResponse, DERControlResponse,
// FlowReservationResponseResponse and DrResponse. So the member of a
// ResponseList is any one of those root elements, and the Mandatory POST on
// /rsps/{id1}/rsp accepts whichever one matches the event being answered.
//
// A single xml.Unmarshal into sep2.Response cannot express that: the pinned
// XMLName rejects every root element but <Response>, so the EPRI reference
// client's <DERControlResponse> was answered with 400. Loosening the pin is
// the wrong repair, because the pin is what stops a <DERSettings> body from
// being silently accepted as a Response. DecodeResponse dispatches on the
// root element instead, and each branch decodes a type whose own XMLName is
// pinned just as tightly.

// responseBody renders a Response subtype document with the given root
// element, written as literal XML rather than marshalled from a struct so the
// test asserts against the wire form a client actually sends.
func responseBody(root, ns string) string {
	return `<` + root + ` xmlns="` + ns + `">` +
		`<createdDateTime>1785429793</createdDateTime>` +
		`<endDeviceLFDI>AABBCCDDEEFF00112233445566778899AABBCCDD</endDeviceLFDI>` +
		`<status>1</status>` +
		`<subject>0123456789ABCDEF0123456789ABCDEF</subject>` +
		`</` + root + `>`
}

// TestDecodeResponse_AcceptsEveryDeclaredSubtype asserts that each root
// element the WADL declares at /rsps/{id1}/rsp/{id2} decodes, and that the
// mandatory Response fields survive with their values intact. endDeviceLFDI
// and subject are minOccurs="1" on Response and are the two fields the server
// matches a response to its event by (subject carries the event mRID), so
// asserting decode succeeded without asserting those values would prove
// nothing about whether the response is usable.
func TestDecodeResponse_AcceptsEveryDeclaredSubtype(t *testing.T) {
	t.Parallel()

	const (
		wantLFDI    = "AABBCCDDEEFF00112233445566778899AABBCCDD"
		wantSubject = "0123456789ABCDEF0123456789ABCDEF"
		wantCreated = int64(1785429793)
		wantStatus  = uint8(1)
	)

	roots := []string{
		"Response",
		"DERControlResponse",
		"FlowReservationResponseResponse",
		"PriceResponse",
		"TextResponse",
	}

	for _, root := range roots {
		t.Run(root, func(t *testing.T) {
			t.Parallel()

			got, err := sep2.DecodeResponse([]byte(responseBody(root, sep2.Namespace)))
			if err != nil {
				t.Fatalf("DecodeResponse(<%s>) = error %v, want it accepted", root, err)
			}
			if got.EndDeviceLFDI != wantLFDI {
				t.Errorf("EndDeviceLFDI = %q, want %q", got.EndDeviceLFDI, wantLFDI)
			}
			if got.Subject != wantSubject {
				t.Errorf("Subject = %q, want %q", got.Subject, wantSubject)
			}
			if got.CreatedDateTime != wantCreated {
				t.Errorf("CreatedDateTime = %d, want %d", got.CreatedDateTime, wantCreated)
			}
			if got.Status == nil {
				t.Fatalf("Status is nil, want %d", wantStatus)
			}
			if *got.Status != wantStatus {
				t.Errorf("Status = %d, want %d", *got.Status, wantStatus)
			}
		})
	}
}

// TestDecodeResponse_NormalizesXMLNameAcrossSubtypes asserts that the
// Response returned for a <DERControlResponse> body marshals to the same
// bytes as the Response returned for a <Response> body.
//
// This is a wire invariant, not tidiness. Every decoded Response is stored in
// one ResponseList, whose members sep.xsd declares as <Response> elements. If
// one decode path left XMLName populated and another left it zero,
// encoding/xml would emit a redundant xmlns on some members and not others,
// so two responses carrying identical data would serialize differently
// depending only on which root element the client happened to POST.
func TestDecodeResponse_NormalizesXMLNameAcrossSubtypes(t *testing.T) {
	t.Parallel()

	base, err := sep2.DecodeResponse([]byte(responseBody("Response", sep2.Namespace)))
	if err != nil {
		t.Fatalf("DecodeResponse(<Response>): %v", err)
	}
	derc, err := sep2.DecodeResponse([]byte(responseBody("DERControlResponse", sep2.Namespace)))
	if err != nil {
		t.Fatalf("DecodeResponse(<DERControlResponse>): %v", err)
	}

	baseBytes, err := xml.Marshal(&base)
	if err != nil {
		t.Fatalf("marshal base: %v", err)
	}
	dercBytes, err := xml.Marshal(&derc)
	if err != nil {
		t.Fatalf("marshal subtype-sourced: %v", err)
	}
	if string(baseBytes) != string(dercBytes) {
		t.Errorf("a Response decoded from <DERControlResponse> serializes differently from one decoded from <Response>\n<Response>:          %s\n<DERControlResponse>: %s",
			baseBytes, dercBytes)
	}
	if !strings.HasPrefix(string(baseBytes), "<Response ") {
		t.Errorf("decoded Response marshals as %s, want a <Response> root", baseBytes)
	}
}

// TestDecodeResponse_RejectsForeignRoots asserts the pin still bites. A body
// whose root element is not a declared Response subtype has to be refused:
// accepting one would store a resource of the wrong kind under the response
// list, and the ResponseSet is the server's record of which events a device
// acknowledged.
func TestDecodeResponse_RejectsForeignRoots(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{"unrelated sep2 type", responseBody("DERSettings", sep2.Namespace)},
		{"wrong namespace", responseBody("Response", "http://example.invalid/ns")},
		{"subtype in wrong namespace", responseBody("DERControlResponse", "http://example.invalid/ns")},
		{"not XML", "this is not xml"},
		{"empty body", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := sep2.DecodeResponse([]byte(tc.body)); err == nil {
				t.Errorf("DecodeResponse(%q) = nil error, want the body refused", tc.body)
			}
		})
	}
}

// TestDecodeResponse_RefusesUnmodelledDrResponse pins the one declared
// subtype this package does not model. DrResponse extends Response with six
// DRLC-specific children (sep.xsd:477-491), none of which this package
// declares, so decoding one into the base would silently discard data the
// client sent. Refusing it is the fail-closed answer, and it costs nothing
// today because the router serves no EndDeviceControl for a DrResponse to
// answer.
func TestDecodeResponse_RefusesUnmodelledDrResponse(t *testing.T) {
	t.Parallel()

	_, err := sep2.DecodeResponse([]byte(responseBody("DrResponse", sep2.Namespace)))
	if err == nil {
		t.Fatal("DecodeResponse(<DrResponse>) = nil error, want it refused as unmodelled")
	}
	if !strings.Contains(err.Error(), "DrResponse") {
		t.Errorf("error %q does not name DrResponse, so an operator cannot tell what was refused", err)
	}
}
