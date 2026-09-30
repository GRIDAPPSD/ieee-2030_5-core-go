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

// TestMirrorMeterReadingElementOrderWithMirrorReadingSet asserts the SERVED
// BYTES place MirrorReadingSet between lastUpdateTime and Reading, per its
// sequence position 5 in MeterReadingBase -> IdentifiedObject (nextUpdateTime,
// position 6, is not implemented). This is the sequence-order half of the
// fix: MirrorReadingSet did not exist as a struct field before, so
// encoding/xml dropped it and left no element to order-check.
func TestMirrorMeterReadingElementOrderWithMirrorReadingSet(t *testing.T) {
	val := int64(5000)
	uom := sep2.UomWatts
	mmr := sep2.MirrorMeterReading{
		MRID:           "MMR01",
		Description:    "Active Power",
		LastUpdateTime: 1700000000,
		MirrorReadingSet: []sep2.MirrorReadingSet{
			{MRID: "SET-1", TimePeriod: sep2.DateTimeInterval{Duration: 900, Start: 1700000000}},
		},
		Reading:     &sep2.Reading{Value: &val},
		ReadingType: &sep2.ReadingType{Uom: &uom},
	}

	data, err := xml.Marshal(&mmr)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)

	wantOrder := []string{"mRID", "description", "lastUpdateTime", "MirrorReadingSet", "Reading", "ReadingType"}
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

// TestMirrorMeterReadingRoundTripPreservesMirrorReadingSet is the
// regression guard for GRIDAPPSD/ieee-2030_5-core-go#191: encoding/xml
// drops any element with no matching struct field, so a MirrorMeterReading
// posted with a MirrorReadingSet (the CSIP aggregator's batch-post shape)
// lost every contained Reading. The fixture carries the example's own
// localID children (Table C.15) and a SECOND MirrorReadingSet, since
// sep.xsd allows maxOccurs="unbounded" and one set surviving does not show
// a sibling would too. Decodes, asserts the decoded values, re-encodes,
// then decodes the RE-ENCODED bytes again and asserts the per-set mRID and
// timePeriod values on THAT struct, so the proof is a value carried through
// a full round trip rather than a substring found in the wire bytes.
func TestMirrorMeterReadingRoundTripPreservesMirrorReadingSet(t *testing.T) {
	body := []byte(`<MirrorMeterReading xmlns="urn:ieee:std:2030.5:ns">
		<mRID>0800006CC8</mRID>
		<MirrorReadingSet>
			<mRID>0900006CC8</mRID>
			<timePeriod>
				<duration>86400</duration>
				<start>1341579365</start>
			</timePeriod>
			<Reading><value>9</value><localID>00</localID></Reading>
			<Reading><value>11</value><localID>01</localID></Reading>
		</MirrorReadingSet>
		<MirrorReadingSet>
			<mRID>0900006CC9</mRID>
			<timePeriod>
				<duration>3600</duration>
				<start>1341665765</start>
			</timePeriod>
			<Reading><value>42</value><localID>00</localID></Reading>
		</MirrorReadingSet>
	</MirrorMeterReading>`)

	var mmr sep2.MirrorMeterReading
	if err := xml.Unmarshal(body, &mmr); err != nil {
		t.Fatal(err)
	}
	assertTwoMirrorReadingSets(t, "decoded", mmr)

	out, err := xml.Marshal(&mmr)
	if err != nil {
		t.Fatal(err)
	}
	reencoded := string(out)

	// Reading declares its own XMLName with the namespace, so encoding/xml
	// repeats xmlns="..." on every element instance: count the opening tag
	// prefix, not a bare "<Reading>".
	if strings.Count(reencoded, "<Reading ") != 3 {
		t.Errorf("re-encoded body lost a Reading, want 3; body=%s", reencoded)
	}
	if strings.Count(reencoded, "<MirrorReadingSet") != 2 {
		t.Errorf("re-encoded body lost a MirrorReadingSet, want 2; body=%s", reencoded)
	}

	firstStart := strings.Index(reencoded, "<MirrorReadingSet")
	if firstStart == -1 {
		t.Fatalf("re-encoded body missing <MirrorReadingSet>; body=%s", reencoded)
	}
	tpIdx := strings.Index(reencoded[firstStart:], "<timePeriod")
	rIdx := strings.Index(reencoded[firstStart:], "<Reading ")
	if tpIdx == -1 || rIdx == -1 || rIdx < tpIdx {
		t.Fatalf("re-encoded MirrorReadingSet did not keep timePeriod before Reading, violates sep.xsd ReadingSetBase sequence order; body=%s",
			reencoded)
	}

	// The strongest proof: decode what was just re-encoded and check the
	// SET-LEVEL values on that fresh struct, not a substring of the bytes
	// that produced it.
	var reDecoded sep2.MirrorMeterReading
	if err := xml.Unmarshal(out, &reDecoded); err != nil {
		t.Fatalf("re-encoded body does not parse: %v; body=%s", err, reencoded)
	}
	assertTwoMirrorReadingSets(t, "re-encoded then re-decoded", reDecoded)
}

// assertTwoMirrorReadingSets asserts the two-MirrorReadingSet fixture's
// field values, shared by the decode assertion and the round-trip
// assertion in TestMirrorMeterReadingRoundTripPreservesMirrorReadingSet so
// both stages are held to the same check.
func assertTwoMirrorReadingSets(t *testing.T, stage string, mmr sep2.MirrorMeterReading) {
	t.Helper()

	if len(mmr.MirrorReadingSet) != 2 {
		t.Fatalf("%s: %d MirrorReadingSet elements, want 2", stage, len(mmr.MirrorReadingSet))
	}

	set0 := mmr.MirrorReadingSet[0]
	if set0.MRID != "0900006CC8" {
		t.Errorf("%s: MirrorReadingSet[0].MRID = %q, want %q", stage, set0.MRID, "0900006CC8")
	}
	if set0.TimePeriod.Duration != 86400 || set0.TimePeriod.Start != 1341579365 {
		t.Errorf("%s: MirrorReadingSet[0].TimePeriod = %+v, want {Duration:86400 Start:1341579365}", stage, set0.TimePeriod)
	}
	if len(set0.Reading) != 2 {
		t.Fatalf("%s: %d Reading elements in MirrorReadingSet[0], want 2", stage, len(set0.Reading))
	}
	if set0.Reading[0].Value == nil || *set0.Reading[0].Value != 9 {
		t.Errorf("%s: MirrorReadingSet[0].Reading[0].Value = %v, want 9", stage, set0.Reading[0].Value)
	}
	if set0.Reading[0].LocalID == nil || *set0.Reading[0].LocalID != 0x00 {
		t.Errorf("%s: MirrorReadingSet[0].Reading[0].LocalID = %v, want 0x00", stage, set0.Reading[0].LocalID)
	}
	if set0.Reading[1].Value == nil || *set0.Reading[1].Value != 11 {
		t.Errorf("%s: MirrorReadingSet[0].Reading[1].Value = %v, want 11", stage, set0.Reading[1].Value)
	}
	if set0.Reading[1].LocalID == nil || *set0.Reading[1].LocalID != 0x01 {
		t.Errorf("%s: MirrorReadingSet[0].Reading[1].LocalID = %v, want 0x01", stage, set0.Reading[1].LocalID)
	}

	set1 := mmr.MirrorReadingSet[1]
	if set1.MRID != "0900006CC9" {
		t.Errorf("%s: MirrorReadingSet[1].MRID = %q, want %q", stage, set1.MRID, "0900006CC9")
	}
	if set1.TimePeriod.Duration != 3600 || set1.TimePeriod.Start != 1341665765 {
		t.Errorf("%s: MirrorReadingSet[1].TimePeriod = %+v, want {Duration:3600 Start:1341665765}", stage, set1.TimePeriod)
	}
	if len(set1.Reading) != 1 {
		t.Fatalf("%s: %d Reading elements in MirrorReadingSet[1], want 1", stage, len(set1.Reading))
	}
	if set1.Reading[0].Value == nil || *set1.Reading[0].Value != 42 {
		t.Errorf("%s: MirrorReadingSet[1].Reading[0].Value = %v, want 42", stage, set1.Reading[0].Value)
	}
}

// TestMirrorMeterReadingListRoundTripPreservesMirrorReadingSet is the list
// half of #191/#192: MirrorMeterReadingList is the actual document a client
// POSTs to a MirrorUsagePoint (section 10.11.3 rule d), so a MirrorReadingSet
// surviving inside a lone MirrorMeterReading does not show it survives
// inside the list wrapper a real POST uses.
func TestMirrorMeterReadingListRoundTripPreservesMirrorReadingSet(t *testing.T) {
	body := []byte(`<MirrorMeterReadingList xmlns="urn:ieee:std:2030.5:ns" all="1" results="1">
		<MirrorMeterReading>
			<mRID>0800006CC8</mRID>
			<MirrorReadingSet>
				<mRID>0900006CC8</mRID>
				<timePeriod>
					<duration>86400</duration>
					<start>1341579365</start>
				</timePeriod>
				<Reading><value>9</value></Reading>
				<Reading><value>11</value></Reading>
			</MirrorReadingSet>
		</MirrorMeterReading>
	</MirrorMeterReadingList>`)

	var list sep2.MirrorMeterReadingList
	if err := xml.Unmarshal(body, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.MirrorMeterReading) != 1 {
		t.Fatalf("decoded %d MirrorMeterReading elements, want 1", len(list.MirrorMeterReading))
	}
	if len(list.MirrorMeterReading[0].MirrorReadingSet) != 1 {
		t.Fatalf("decoded %d MirrorReadingSet elements, want 1", len(list.MirrorMeterReading[0].MirrorReadingSet))
	}
	if len(list.MirrorMeterReading[0].MirrorReadingSet[0].Reading) != 2 {
		t.Fatalf("decoded %d Reading elements, want 2", len(list.MirrorMeterReading[0].MirrorReadingSet[0].Reading))
	}

	out, err := xml.Marshal(&list)
	if err != nil {
		t.Fatal(err)
	}

	var reDecoded sep2.MirrorMeterReadingList
	if err := xml.Unmarshal(out, &reDecoded); err != nil {
		t.Fatalf("re-encoded list does not parse: %v; body=%s", err, out)
	}
	if len(reDecoded.MirrorMeterReading) != 1 {
		t.Fatalf("re-encoded list lost the MirrorMeterReading, want 1; body=%s", out)
	}
	if len(reDecoded.MirrorMeterReading[0].MirrorReadingSet) != 1 {
		t.Fatalf("re-encoded list lost the MirrorReadingSet, want 1; body=%s", out)
	}
	rs := reDecoded.MirrorMeterReading[0].MirrorReadingSet[0]
	if rs.MRID != "0900006CC8" {
		t.Errorf("re-encoded list: MirrorReadingSet.MRID = %q, want %q; body=%s", rs.MRID, "0900006CC8", out)
	}
	if len(rs.Reading) != 2 {
		t.Fatalf("re-encoded list lost a Reading, want 2; body=%s", out)
	}
	if rs.Reading[0].Value == nil || *rs.Reading[0].Value != 9 {
		t.Errorf("re-encoded list: Reading[0].Value = %v, want 9; body=%s", rs.Reading[0].Value, out)
	}
	if rs.Reading[1].Value == nil || *rs.Reading[1].Value != 11 {
		t.Errorf("re-encoded list: Reading[1].Value = %v, want 11; body=%s", rs.Reading[1].Value, out)
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
