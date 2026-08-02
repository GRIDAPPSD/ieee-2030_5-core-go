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

// TestReadingTypeNoMRID asserts the served bytes never carry an mRID
// element inside a ReadingType. This is the regression guard for the root
// cause: sep.xsd's ReadingType is <xs:extension base="Resource"/> only, and
// Resource contributes solely the href attribute, no elements. A prior
// version of this struct emitted mRID as ReadingType's first child, which
// does not exist in the EPRI reference client's schema table
// (se_schema.c, ReadingType (330): href, accumulationBehaviour, ...), so
// the client's strict parser rejected the whole document. Checked both
// standalone and nested inside a MirrorMeterReading, since the nested path
// is how a device actually receives it.
func TestReadingTypeNoMRID(t *testing.T) {
	uom := sep2.UomWatts
	rt := sep2.ReadingType{
		Resource: sep2.Resource{Href: "/mup/inv1/mr/1/rt"},
		Uom:      &uom,
	}

	data, err := xml.Marshal(&rt)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if strings.Contains(body, "<mRID") {
		t.Errorf("served ReadingType carries an mRID element, which does not exist in sep.xsd's ReadingType sequence; body=%s", body)
	}

	val := int64(5000)
	mmr := sep2.MirrorMeterReading{
		Resource: sep2.Resource{Href: "/mup/inv1/mr/1"},
		MRID:     "MMR01",
		Reading:  &sep2.Reading{Value: &val},
		ReadingType: &sep2.ReadingType{
			Uom: &uom,
		},
	}
	data, err = xml.Marshal(&mmr)
	if err != nil {
		t.Fatal(err)
	}
	body = string(data)

	rtStart := strings.Index(body, "<ReadingType")
	if rtStart == -1 {
		t.Fatalf("served MirrorMeterReading missing <ReadingType>; body=%s", body)
	}
	rtEnd := strings.Index(body[rtStart:], "</ReadingType>")
	if rtEnd == -1 {
		t.Fatalf("served MirrorMeterReading missing closing </ReadingType>; body=%s", body)
	}
	readingTypeElement := body[rtStart : rtStart+rtEnd]
	if strings.Contains(readingTypeElement, "<mRID") {
		t.Errorf("served ReadingType (nested in MirrorMeterReading) carries an mRID element; readingTypeElement=%s, full body=%s", readingTypeElement, body)
	}
}

// TestMirrorMeterReadingElementOrder asserts the SERVED BYTES carry
// MirrorMeterReading's children in the sep.xsd sequence order (mRID,
// description, lastUpdateTime, Reading, ReadingType), with both Reading
// and ReadingType present. This was latent while the only client behavior
// was posting ReadingType alone; it breaks a strict sequence-validating
// parser the moment both are present, per sep.xsd:6416 (MirrorMeterReading
// -> MeterReadingBase -> IdentifiedObject), which places Reading (position
// 7) before ReadingType (position 8).
func TestMirrorMeterReadingElementOrder(t *testing.T) {
	val := int64(5000)
	uom := sep2.UomWatts
	mmr := sep2.MirrorMeterReading{
		Resource:       sep2.Resource{Href: "/mup/inv1/mr/1"},
		MRID:           "MMR01",
		Description:    "Active Power",
		LastUpdateTime: 1700000000,
		Reading:        &sep2.Reading{Value: &val},
		ReadingType:    &sep2.ReadingType{Uom: &uom},
	}

	data, err := xml.Marshal(&mmr)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)

	wantOrder := []string{"mRID", "description", "lastUpdateTime", "Reading", "ReadingType"}
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

// TestMirrorMeterReadingMRIDAlwaysEmitted asserts MRID serializes even at
// its zero value. IdentifiedObject (sep.xsd:5324) declares mRID
// minOccurs="1"; "omitempty" on a string field silently drops the element
// for the empty string, which would serve a document missing a required
// element.
func TestMirrorMeterReadingMRIDAlwaysEmitted(t *testing.T) {
	mmr := sep2.MirrorMeterReading{Resource: sep2.Resource{Href: "/mup/inv1/mr/1"}}

	data, err := xml.Marshal(&mmr)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, "<mRID></mRID>") {
		t.Errorf("served MirrorMeterReading at zero-value MRID missing <mRID></mRID>; body=%s", body)
	}
}

// TestReadingElementOrder asserts the SERVED BYTES carry Reading's
// implemented children in the ReadingBase sequence order (qualityFlags,
// timePeriod, value), per sep.xsd:6511. A prior version of this struct
// declared value first, which is out of order relative to qualityFlags
// (position 2) and timePeriod (position 3): any served Reading with more
// than one of these fields set would fail a strict sequence-validating
// parser.
func TestReadingElementOrder(t *testing.T) {
	val := int64(5000)
	qf := sep2.HexBinary16(0x0009)
	reading := sep2.Reading{
		Resource:     sep2.Resource{Href: "/mup/inv1/mr/1/r"},
		QualityFlags: &qf,
		TimePeriod:   &sep2.DateTimeInterval{Duration: 900, Start: 1700000000},
		Value:        &val,
	}

	data, err := xml.Marshal(&reading)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)

	wantOrder := []string{"qualityFlags", "timePeriod", "value"}
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
			t.Fatalf("element %q (pos %d) appears before %q (pos %d), violates sep.xsd ReadingBase sequence order; body=%s",
				wantOrder[i], positions[i], wantOrder[i-1], positions[i-1], body)
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
