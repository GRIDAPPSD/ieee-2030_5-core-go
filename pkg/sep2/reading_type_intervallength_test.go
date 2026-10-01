package sep2_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

const mirrorMeterReadingWithIntervalLength = `<MirrorMeterReading xmlns="urn:ieee:std:2030.5:ns" href="/mup/1/mr/1">` +
	`<mRID>MMR01</mRID>` +
	`<ReadingType><flowDirection>1</flowDirection><intervalLength>900</intervalLength><kind>37</kind><uom>38</uom></ReadingType>` +
	`</MirrorMeterReading>`

// Decoding a mirror reading body must surface intervalLength in seconds.
func TestReadingTypeDecodesIntervalLength(t *testing.T) {
	var mmr sep2.MirrorMeterReading
	if err := xml.Unmarshal([]byte(mirrorMeterReadingWithIntervalLength), &mmr); err != nil {
		t.Fatal(err)
	}
	rt := mmr.ReadingType
	if rt == nil || rt.IntervalLength == nil {
		t.Fatalf("IntervalLength not decoded: %+v", rt)
	}
	if *rt.IntervalLength != 900 {
		t.Errorf("IntervalLength = %d, want 900", *rt.IntervalLength)
	}
	if rt.Kind == nil || *rt.Kind != 37 || rt.Uom == nil || *rt.Uom != 38 {
		t.Errorf("neighbouring fields disturbed: kind=%v uom=%v", rt.Kind, rt.Uom)
	}
}

// The served bytes carry intervalLength between flowDirection and kind, and
// an absent value emits no element.
func TestReadingTypeIntervalLengthWire(t *testing.T) {
	flow := sep2.FlowDirectionForward
	kind := uint8(37)
	il := uint32(900)
	rt := sep2.ReadingType{FlowDirection: &flow, IntervalLength: &il, Kind: &kind}
	data, err := xml.Marshal(&rt)
	if err != nil {
		t.Fatal(err)
	}
	want := "<flowDirection>1</flowDirection><intervalLength>900</intervalLength><kind>37</kind>"
	if !strings.Contains(string(data), want) {
		t.Errorf("wire order wrong: %s", data)
	}

	data, err = xml.Marshal(&sep2.ReadingType{Kind: &kind})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "intervalLength") {
		t.Errorf("absent IntervalLength emitted: %s", data)
	}
}

// A decoded body re-encodes to the same intervalLength, and Copy does not alias it.
func TestReadingTypeIntervalLengthRoundTripAndCopy(t *testing.T) {
	var mmr sep2.MirrorMeterReading
	if err := xml.Unmarshal([]byte(mirrorMeterReadingWithIntervalLength), &mmr); err != nil {
		t.Fatal(err)
	}
	data, err := xml.Marshal(&mmr)
	if err != nil {
		t.Fatal(err)
	}
	var back sep2.MirrorMeterReading
	if err := xml.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.ReadingType == nil || back.ReadingType.IntervalLength == nil || *back.ReadingType.IntervalLength != 900 {
		t.Fatalf("round trip lost intervalLength: %s", data)
	}

	c := back.ReadingType.Copy()
	*c.IntervalLength = 1
	if *back.ReadingType.IntervalLength != 900 {
		t.Error("Copy aliased IntervalLength")
	}
}
