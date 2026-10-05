package sep2_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/internal/xsdgate"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// Documents use populated, distinct values so a dropped or swapped field
// fails on the decoded value rather than matching a zero.
const derStatus2018Stor = `<DERStatus xmlns="urn:ieee:std:2030.5:ns">` +
	`<genConnectStatus><dateTime>100</dateTime><value>01</value></genConnectStatus>` +
	`<readingTime>1604963587</readingTime>` +
	`<storConnectStatus><dateTime>200</dateTime><value>0B</value></storConnectStatus>` +
	`</DERStatus>`

const derStatus2023Connect = `<DERStatus xmlns="urn:ieee:std:2030.5:ns">` +
	`<connectStatus><dateTime>300</dateTime><value>03</value></connectStatus>` +
	`<readingTime>1604963587</readingTime>` +
	`</DERStatus>`

func TestDERStatusDecodesStorConnectStatus(t *testing.T) {
	var s sep2.DERStatus
	if err := xml.Unmarshal([]byte(derStatus2018Stor), &s); err != nil {
		t.Fatal(err)
	}
	if s.StorConnectStatus == nil {
		t.Fatal("storConnectStatus dropped on decode")
	}
	if s.StorConnectStatus.DateTime != 200 || s.StorConnectStatus.Value != 0x0B {
		t.Errorf("storConnectStatus = %+v, want dateTime 200 value 0x0B", *s.StorConnectStatus)
	}
	if s.GenConnectStatus == nil || s.GenConnectStatus.DateTime != 100 || s.GenConnectStatus.Value != 0x01 {
		t.Errorf("genConnectStatus = %+v, want dateTime 100 value 0x01", s.GenConnectStatus)
	}
	if s.ConnectStatus != nil {
		t.Errorf("connectStatus = %+v, want nil for a 2018 document", *s.ConnectStatus)
	}
}

func TestDERStatusDecodesConnectStatus2023(t *testing.T) {
	var s sep2.DERStatus
	if err := xml.Unmarshal([]byte(derStatus2023Connect), &s); err != nil {
		t.Fatal(err)
	}
	if s.ConnectStatus == nil {
		t.Fatal("connectStatus dropped on decode")
	}
	if s.ConnectStatus.DateTime != 300 || s.ConnectStatus.Value != 0x03 {
		t.Errorf("connectStatus = %+v, want dateTime 300 value 0x03 (connected, energized)", *s.ConnectStatus)
	}
	if s.GenConnectStatus != nil || s.StorConnectStatus != nil {
		t.Errorf("2018 connect fields set on a 2023 document: gen=%v stor=%v", s.GenConnectStatus, s.StorConnectStatus)
	}
}

func TestDERStatusConnectStatusRoundTripAndOrder(t *testing.T) {
	in := sep2.DERStatus{
		ConnectStatus:     &sep2.ConnectStatusType2{DateTime: 300, Value: 0x03},
		GenConnectStatus:  &sep2.ConnectStatusType{DateTime: 100, Value: 0x01},
		StorConnectStatus: &sep2.ConnectStatusType{DateTime: 200, Value: 0x0B},
		ReadingTime:       1604963587,
	}
	data, err := xml.Marshal(&in)
	if err != nil {
		t.Fatal(err)
	}
	// 2023 sequence: connectStatus precedes genConnectStatus; 2018 and 2023
	// both end the connect elements with storConnectStatus after readingTime.
	assertOrder(t, string(data), []string{
		"<connectStatus>", "<genConnectStatus>", "<readingTime>", "<storConnectStatus>",
	})
	if !strings.Contains(string(data), "<storConnectStatus><dateTime>200</dateTime><value>0B</value></storConnectStatus>") {
		t.Errorf("storConnectStatus wire form wrong: %s", data)
	}
	if !strings.Contains(string(data), "<connectStatus><dateTime>300</dateTime><value>03</value></connectStatus>") {
		t.Errorf("connectStatus wire form wrong: %s", data)
	}

	var out sep2.DERStatus
	if err := xml.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if *out.ConnectStatus != *in.ConnectStatus || *out.GenConnectStatus != *in.GenConnectStatus ||
		*out.StorConnectStatus != *in.StorConnectStatus {
		t.Errorf("round trip changed values: got %+v %+v %+v", *out.ConnectStatus, *out.GenConnectStatus, *out.StorConnectStatus)
	}
}

func TestDERStatusOmitsAbsentConnectFields(t *testing.T) {
	data, err := xml.Marshal(&sep2.DERStatus{ReadingTime: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, el := range []string{"connectStatus", "storConnectStatus", "genConnectStatus"} {
		if strings.Contains(string(data), el) {
			t.Errorf("%s emitted for a nil field: %s", el, data)
		}
	}
}

func TestDERStatusCopyConnectFields(t *testing.T) {
	orig := sep2.DERStatus{
		ConnectStatus:     &sep2.ConnectStatusType2{DateTime: 300, Value: 0x03},
		StorConnectStatus: &sep2.ConnectStatusType{DateTime: 200, Value: 0x0B},
	}
	c := orig.Copy()
	c.ConnectStatus.Value = 0
	c.StorConnectStatus.Value = 0
	if orig.ConnectStatus.Value != 0x03 || orig.StorConnectStatus.Value != 0x0B {
		t.Errorf("Copy aliased connect fields: %+v %+v", *orig.ConnectStatus, *orig.StorConnectStatus)
	}
}

// The schema gate pins the 2018 edition, so only storConnectStatus is
// validated against it; connectStatus is 2023-only.
func TestDERStatusStorConnectStatusSchemaValid(t *testing.T) {
	xsdgate.AssertValid(t, "DERStatus", &sep2.DERStatus{
		ReadingTime:       1604963587,
		StorConnectStatus: &sep2.ConnectStatusType{DateTime: 200, Value: 0x0B},
	})
}
