package sep2_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// listXML wraps DERControl items in a DERControlList that carries list
// attributes, so the tests also check the list fields survive.
func listXML(items ...string) string {
	return `<DERControlList xmlns="urn:ieee:std:2030.5:ns" href="/derp/1/derc" all="2" results="2">` +
		strings.Join(items, "") + `</DERControlList>`
}

func controlXML(mrid, replyTo, responseRequired, base string) string {
	return `<DERControl replyTo="` + replyTo + `" responseRequired="` + responseRequired + `">` +
		`<mRID>` + mrid + `</mRID><creationTime>1700000000</creationTime>` +
		`<EventStatus><currentStatus>0</currentStatus><dateTime>1700000000</dateTime><potentiallySuperseded>false</potentiallySuperseded></EventStatus>` +
		`<interval><duration>600</duration><start>1700000600</start></interval>` +
		`<DERControlBase>` + base + `</DERControlBase></DERControl>`
}

const validFixedVar = `<opModFixedVar><refType>1</refType><value>5000</value></opModFixedVar>`

func TestDERControlListResultRejectsBadBaseItem(t *testing.T) {
	cases := []struct {
		name      string
		base      string
		errSubstr string
	}{
		{"FixedVar refType 4", `<opModFixedVar><refType>4</refType><value>5000</value></opModFixedVar>`, "refType 4"},
		{"FixedVar refType 0", `<opModFixedVar><refType>0</refType><value>5000</value></opModFixedVar>`, "refType 0"},
		{"FixedVar value 10001", `<opModFixedVar><refType>1</refType><value>10001</value></opModFixedVar>`, "FixedVar.value"},
		{"opModMaxLimW 10001", `<opModMaxLimW>10001</opModMaxLimW>`, "PerCent 10001"},
		{"opModFixedW -10001", `<opModFixedW>-10001</opModFixedW>`, "SignedPerCent -10001"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data := listXML(
				controlXML("A1", "/rsp/a", "01", validFixedVar),
				controlXML("B2", "/rsp/b", "03", tc.base),
			)

			var res sep2.DERControlListResult
			if err := xml.Unmarshal([]byte(data), &res); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			if len(res.DERControl) != 1 {
				t.Fatalf("controls = %d, want 1", len(res.DERControl))
			}
			got := res.DERControl[0]
			if got.MRID != "A1" || got.ReplyTo != "/rsp/a" {
				t.Errorf("control identity = %q %q, want A1 /rsp/a", got.MRID, got.ReplyTo)
			}
			if got.DERControlBase == nil || got.DERControlBase.OpModFixedVar == nil ||
				*got.DERControlBase.OpModFixedVar != (sep2.FixedVar{RefType: 1, Value: 5000}) {
				t.Errorf("control base = %+v, want FixedVar{1 5000}", got.DERControlBase)
			}

			if len(res.Rejected) != 1 {
				t.Fatalf("rejected = %d, want 1", len(res.Rejected))
			}
			rej := res.Rejected[0]
			if rej.MRID != "B2" {
				t.Errorf("rejected mRID = %q, want B2", rej.MRID)
			}
			if rej.ReplyTo != "/rsp/b" {
				t.Errorf("rejected replyTo = %q, want /rsp/b", rej.ReplyTo)
			}
			if rej.ResponseRequired == nil || *rej.ResponseRequired != 0x03 {
				t.Errorf("rejected responseRequired = %v, want 0x03", rej.ResponseRequired)
			}
			if rej.CreationTime != 1700000000 || rej.Interval == nil || rej.Interval.Start != 1700000600 {
				t.Errorf("rejected event fields = creationTime %d interval %+v", rej.CreationTime, rej.Interval)
			}
			if rej.Err == nil || !strings.Contains(rej.Err.Error(), tc.errSubstr) {
				t.Errorf("rejected error = %v, want it to name %q", rej.Err, tc.errSubstr)
			}
			for _, c := range res.DERControl {
				if c.MRID == rej.MRID {
					t.Errorf("rejected mRID %q also appears among the controls", rej.MRID)
				}
			}

			if res.Href != "/derp/1/derc" || res.All != 2 || res.Results != 2 {
				t.Errorf("list fields = href %q all %d results %d, want /derp/1/derc 2 2",
					res.Href, res.All, res.Results)
			}
		})
	}
}

func TestDERControlListResultRefTypeErrorNamesField(t *testing.T) {
	t.Parallel()
	data := listXML(controlXML("B2", "/rsp/b", "03",
		`<opModFixedVar><refType>4</refType><value>5000</value></opModFixedVar>`))
	var res sep2.DERControlListResult
	if err := xml.Unmarshal([]byte(data), &res); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(res.Rejected) != 1 || !strings.Contains(res.Rejected[0].Err.Error(), "refType") {
		t.Fatalf("rejected = %+v, want one item whose error names refType", res.Rejected)
	}
	if len(res.DERControl) != 0 {
		t.Errorf("controls = %d, want 0", len(res.DERControl))
	}
}

func TestDERControlListResultAllValid(t *testing.T) {
	t.Parallel()
	data := listXML(
		controlXML("A1", "/rsp/a", "01", validFixedVar),
		controlXML("A2", "/rsp/b", "00", `<opModMaxLimW>8000</opModMaxLimW>`),
	)
	var res sep2.DERControlListResult
	if err := xml.Unmarshal([]byte(data), &res); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(res.Rejected) != 0 {
		t.Errorf("rejected = %+v, want none", res.Rejected)
	}
	if len(res.DERControl) != 2 || res.DERControl[0].MRID != "A1" || res.DERControl[1].MRID != "A2" {
		t.Fatalf("controls = %+v, want A1 then A2", res.DERControl)
	}
	if lim := res.DERControl[1].DERControlBase.OpModMaxLimW; lim == nil || *lim != 8000 {
		t.Errorf("A2 opModMaxLimW = %v, want 8000", lim)
	}

	// The result decodes the same controls as the strict list does.
	var strict sep2.DERControlList
	if err := xml.Unmarshal([]byte(data), &strict); err != nil {
		t.Fatalf("strict Unmarshal: %v", err)
	}
	if len(strict.DERControl) != len(res.DERControl) {
		t.Fatalf("strict controls = %d, result controls = %d", len(strict.DERControl), len(res.DERControl))
	}
	for i := range strict.DERControl {
		a, b := strict.DERControl[i], res.DERControl[i]
		if a.MRID != b.MRID || a.ReplyTo != b.ReplyTo || *a.ResponseRequired != *b.ResponseRequired ||
			a.CreationTime != b.CreationTime || *a.Interval != *b.Interval {
			t.Errorf("item %d differs: strict %+v result %+v", i, a, b)
		}
	}
}

func TestDERControlListResultEventFieldFailureFailsWholeDecode(t *testing.T) {
	cases := []struct {
		name string
		item string
	}{
		{"bad responseRequired, valid base", controlXML("B2", "/rsp/b", "ZZ", validFixedVar)},
		{"bad responseRequired, bad base", controlXML("B2", "/rsp/b", "ZZ",
			`<opModFixedVar><refType>4</refType><value>5000</value></opModFixedVar>`)},
		{"bad randomizeStart", strings.Replace(controlXML("B2", "/rsp/b", "01", validFixedVar),
			`<DERControlBase>`, `<randomizeStart>3601</randomizeStart><DERControlBase>`, 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data := listXML(controlXML("A1", "/rsp/a", "01", validFixedVar), tc.item)
			var res sep2.DERControlListResult
			if err := xml.Unmarshal([]byte(data), &res); err == nil {
				t.Fatalf("Unmarshal succeeded with controls %d rejected %d, want an error",
					len(res.DERControl), len(res.Rejected))
			}
		})
	}
}

func TestDERControlListStillStrict(t *testing.T) {
	t.Parallel()
	data := listXML(
		controlXML("A1", "/rsp/a", "01", validFixedVar),
		controlXML("B2", "/rsp/b", "03", `<opModFixedVar><refType>4</refType><value>5000</value></opModFixedVar>`),
	)
	var list sep2.DERControlList
	err := xml.Unmarshal([]byte(data), &list)
	if err == nil || !strings.Contains(err.Error(), "refType 4") {
		t.Fatalf("DERControlList Unmarshal err = %v, want the refType 4 error", err)
	}
}

func TestDERControlListResultWrongRoot(t *testing.T) {
	t.Parallel()
	data := `<DERControl xmlns="urn:ieee:std:2030.5:ns"><mRID>A1</mRID><creationTime>1</creationTime></DERControl>`
	var res sep2.DERControlListResult
	if err := xml.Unmarshal([]byte(data), &res); err == nil {
		t.Fatal("Unmarshal of a DERControl root succeeded, want an error")
	}
}

func TestDERControlListResultMarshalMatchesList(t *testing.T) {
	t.Parallel()
	data := listXML(
		controlXML("A1", "/rsp/a", "01", validFixedVar),
		controlXML("B2", "/rsp/b", "03", `<opModFixedVar><refType>4</refType><value>5000</value></opModFixedVar>`),
	)
	var res sep2.DERControlListResult
	if err := xml.Unmarshal([]byte(data), &res); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	got, err := xml.Marshal(res)
	if err != nil {
		t.Fatalf("Marshal result: %v", err)
	}
	want, err := xml.Marshal(res.DERControlList)
	if err != nil {
		t.Fatalf("Marshal list: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("result marshals as\n%s\nwant the list's encoding\n%s", got, want)
	}
}
