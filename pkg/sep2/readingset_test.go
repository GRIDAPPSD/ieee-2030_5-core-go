package sep2_test

import (
	"encoding/xml"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

func sampleReadingSet() sep2.ReadingSet {
	ver := uint16(3)
	return sep2.ReadingSet{
		Resource: sep2.Resource{Href: "/upt/1/mr/1/rs/1"},
		IdentifiedObject: sep2.IdentifiedObject{
			MRID:        "0A0000000001",
			Description: "hour 1",
			Version:     &ver,
		},
		TimePeriod:      sep2.DateTimeInterval{Duration: 3600, Start: 1341579365},
		ReadingListLink: &sep2.ListLink{Href: "/upt/1/mr/1/rs/1/r", All: 4},
	}
}

// TestReadingSetWireOrder pins the sep.xsd ReadingSet sequence: mRID,
// description, version, timePeriod, ReadingListLink.
func TestReadingSetWireOrder(t *testing.T) {
	rs := sampleReadingSet()
	data, err := xml.Marshal(&rs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	assertOrder(t, string(data), []string{
		"<mRID>", "<description>", "<version>", "<timePeriod>", "<ReadingListLink",
	})
}

// TestReadingSetListRoundTrip marshals, unmarshals and marshals again,
// comparing the bytes and the decoded field values.
func TestReadingSetListRoundTrip(t *testing.T) {
	list := sep2.ReadingSetList{
		ListResource: sep2.ListResource{
			SubscribableResource: sep2.SubscribableResource{Resource: sep2.Resource{Href: "/upt/1/mr/1/rs"}},
			All:                  1, Results: 1,
		},
		ReadingSet: []sep2.ReadingSet{sampleReadingSet()},
	}
	first, err := xml.Marshal(&list)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back sep2.ReadingSetList
	if err := xml.Unmarshal(first, &back); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, first)
	}
	if len(back.ReadingSet) != 1 {
		t.Fatalf("decoded %d ReadingSet, want 1; body=%s", len(back.ReadingSet), first)
	}
	got := back.ReadingSet[0]
	if got.MRID != "0A0000000001" || got.Description != "hour 1" ||
		got.Version == nil || *got.Version != 3 ||
		got.TimePeriod.Duration != 3600 || got.TimePeriod.Start != 1341579365 ||
		got.ReadingListLink == nil || got.ReadingListLink.Href != "/upt/1/mr/1/rs/1/r" || got.ReadingListLink.All != 4 {
		t.Errorf("decoded ReadingSet lost a field: %+v", got)
	}
	second, err := xml.Marshal(&back)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if string(first) != string(second) {
		t.Errorf("round trip changed bytes:\n first  %s\n second %s", first, second)
	}
}
