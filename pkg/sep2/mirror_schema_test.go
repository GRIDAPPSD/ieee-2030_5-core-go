package sep2_test

import (
	"encoding/xml"
	"regexp"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// TestMirrorUsagePointElementOrder asserts the SERVED BYTES carry
// MirrorUsagePoint's children in the sep.xsd sequence order (mRID,
// description, roleFlags, serviceCategoryKind, status, deviceLFDI,
// MirrorMeterReading, postRate), not merely that marshal does not error.
// A schema-validating xs:sequence parser (the EPRI reference client among
// them) rejects the whole document on an out-of-order child, so proving
// "no crash" proves nothing about wire compatibility.
func TestMirrorUsagePointElementOrder(t *testing.T) {
	rate := uint32(300)
	mup := sep2.MirrorUsagePoint{
		Resource:            sep2.Resource{Href: "/mup/inv1"},
		MRID:                "INV001",
		Description:         "Inverter 1",
		RoleFlags:           sep2.RoleFlagsValue(9),
		ServiceCategoryKind: 0,
		Status:              1,
		DeviceLFDI:          "ABCDEF0123456789ABCD",
		MirrorMeterReading:  []sep2.MirrorMeterReading{{MRID: "MMR01"}},
		PostRate:            &rate,
	}

	data, err := xml.Marshal(&mup)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)

	wantOrder := []string{
		"mRID", "description", "roleFlags", "serviceCategoryKind",
		"status", "deviceLFDI", "MirrorMeterReading", "postRate",
	}

	positions := make([]int, len(wantOrder))
	for i, tag := range wantOrder {
		idx := strings.Index(body, "<"+tag)
		if idx == -1 {
			t.Fatalf("served bytes missing <%s>; body=%s", tag, body)
		}
		positions[i] = idx
	}
	for i := 1; i < len(positions); i++ {
		if positions[i] < positions[i-1] {
			t.Fatalf("element %q (pos %d) appears before %q (pos %d), violates sep.xsd sequence order; body=%s",
				wantOrder[i], positions[i], wantOrder[i-1], positions[i-1], body)
		}
	}
}

// TestMirrorUsagePointRoleFlagsHexBinaryPadding asserts the served bytes
// carry roleFlags as even-length hex text for every value below 0x10, per
// sep.xsd's HexBinary16 octet-pairing rule ("hexBinary requires pairs of
// hex characters, so an odd number of characters requires a leading 0",
// sep.xsd:6249). A one-nibble "9" is not legal hexBinary text at all.
func TestMirrorUsagePointRoleFlagsHexBinaryPadding(t *testing.T) {
	cases := []struct {
		value sep2.RoleFlagsValue
		want  string
	}{
		{0, "00"},
		{9, "09"}, // isMirror | isDER, the EPRI client's actual value
		{15, "0F"},
		{16, "10"},    // already even-length; must not be double-padded
		{300, "012C"}, // 0x12C, three hex digits, pad to four
	}

	for _, c := range cases {
		mup := sep2.MirrorUsagePoint{RoleFlags: c.value, ServiceCategoryKind: 0, Status: 1}
		data, err := xml.Marshal(&mup)
		if err != nil {
			t.Fatalf("value=%d: marshal error: %v", c.value, err)
		}
		body := string(data)

		re := regexp.MustCompile(`<roleFlags>([0-9A-Fa-f]*)</roleFlags>`)
		m := re.FindStringSubmatch(body)
		if m == nil {
			t.Fatalf("value=%d: served bytes missing <roleFlags> text element; body=%s", c.value, body)
		}
		got := m[1]
		if len(got)%2 != 0 {
			t.Errorf("value=%d: served roleFlags text %q has odd digit count, violates HexBinary16 octet-pairing rule; body=%s", c.value, got, body)
		}
		if got != c.want {
			t.Errorf("value=%d: served roleFlags text = %q, want %q; body=%s", c.value, got, c.want, body)
		}
	}
}

// TestMirrorUsagePointNoMeterReadingListLink asserts the served bytes never
// carry a MirrorMeterReadingListLink element. sep.xsd defines no such type
// for MirrorUsagePoint (grep of the full schema element index returns zero
// matches); a schema-validating client rejects any element it cannot
// resolve to a declared type.
func TestMirrorUsagePointNoMeterReadingListLink(t *testing.T) {
	mup := sep2.MirrorUsagePoint{
		Resource:           sep2.Resource{Href: "/mup/inv1"},
		MRID:               "INV001",
		MirrorMeterReading: []sep2.MirrorMeterReading{{MRID: "MMR01"}},
	}
	data, err := xml.Marshal(&mup)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if strings.Contains(body, "MirrorMeterReadingListLink") {
		t.Errorf("served MirrorUsagePoint carries undefined MirrorMeterReadingListLink; body=%s", body)
	}
}
