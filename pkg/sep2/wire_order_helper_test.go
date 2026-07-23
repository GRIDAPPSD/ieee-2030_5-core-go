package sep2_test

import (
	"strings"
	"testing"
)

// assertOrder asserts that each of wantOrder's elements appears in xmlStr,
// and that their byte-index positions are strictly increasing: i.e. it
// asserts the ACTUAL wire element order, not merely each element's presence.
// This is the ordering-invariant check required by IEEE 2030.5's
// order-significant xsd:sequence complex types: a strict client (e.g. EPRI
// oeg_client) validates the schema sequence and rejects out-of-order XML.
func assertOrder(t *testing.T, xmlStr string, wantOrder []string) {
	t.Helper()
	lastIdx := -1
	lastName := ""
	for _, elem := range wantOrder {
		idx := strings.Index(xmlStr, elem)
		if idx == -1 {
			t.Fatalf("element %q not found in XML:\n%s", elem, xmlStr)
		}
		if idx <= lastIdx {
			t.Fatalf("element %q (index %d) does not appear after %q (index %d); wire order violates XSD sequence:\n%s", elem, idx, lastName, lastIdx, xmlStr)
		}
		lastIdx = idx
		lastName = elem
	}
}
