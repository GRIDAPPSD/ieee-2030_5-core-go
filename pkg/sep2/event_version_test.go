package sep2_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

func u16(v uint16) *uint16 { return &v }

// version is a child element of Event, declared minOccurs=0 after
// description and before creationTime (sep.xsd:5474 and sep.xsd:5571).
func TestEventVersionWireBytes(t *testing.T) {
	ev := sep2.DERControl{}
	ev.MRID = "0102030405060708090A0B0C0D0E0F10"
	ev.Description = "d"
	ev.Version = u16(3)
	ev.CreationTime = 1500000000

	data, err := xml.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	want := "<mRID>0102030405060708090A0B0C0D0E0F10</mRID><description>d</description><version>3</version><creationTime>1500000000</creationTime>"
	if !strings.Contains(s, want) {
		t.Errorf("version must sit between description and creationTime as an element:\nwant substring %s\ngot %s", want, s)
	}
	if strings.Contains(s, " version=") {
		t.Errorf("version emitted as an attribute: %s", s)
	}
}

func TestEventVersionZeroIsDistinctFromAbsent(t *testing.T) {
	ev := sep2.TextMessage{}
	ev.Version = u16(0)
	data, err := xml.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "<version>0</version>") {
		t.Errorf("explicit version 0 must reach the wire: %s", data)
	}
	data, err = xml.Marshal(sep2.TextMessage{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "<version>") {
		t.Errorf("nil version must be absent: %s", data)
	}
}

func TestEventVersionDecodes(t *testing.T) {
	in := `<DERControl xmlns="urn:ieee:std:2030.5:ns"><mRID>AB</mRID><version>65535</version><creationTime>1</creationTime></DERControl>`
	var d sep2.DERControl
	if err := xml.Unmarshal([]byte(in), &d); err != nil {
		t.Fatal(err)
	}
	if d.Version == nil || *d.Version != 65535 {
		t.Errorf("Version = %v, want 65535", d.Version)
	}
}

func TestEventCopyDoesNotAliasVersion(t *testing.T) {
	d := sep2.DERControl{}
	d.Version = u16(7)
	c := d.Copy()
	*d.Version = 8
	if c.Version == nil || *c.Version != 7 {
		t.Errorf("copy aliases Version: got %v, want 7", c.Version)
	}
	e := sep2.Event{Version: u16(1)}
	ec := e.Copy()
	*e.Version = 2
	if *ec.Version != 1 {
		t.Errorf("Event.Copy aliases Version: %d", *ec.Version)
	}
}
