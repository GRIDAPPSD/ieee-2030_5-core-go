package sep2_test

import (
	"encoding/xml"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// These tests assert SERVED BYTES, not struct values, and that is the whole
// point. The defect they lock out never corrupted the Go struct: a
// modesSupported field holding 2048 always held 2048. What was wrong was the
// serialization. encoding/xml rendered the uint32 as the decimal text
// "2048", and sep.xsd types that element as hexBinary (DERControlType
// extends HexBinary32, sep.xsd:3952), so a conforming peer read the text as
// hex and got 0x2048, which is 8264. The EPRI reference client does exactly
// that (xml_parse.c:86 dispatches XS_HEX_BINARY to parse_hex) and raises no
// error while doing it, because "2048" is perfectly valid hexBinary text.
// A struct-level assertion cannot see any of this. Only the bytes can.

// hexBinaryElementText returns the text content of the named element in body,
// failing the test when the element is absent.
func hexBinaryElementText(t *testing.T, body, name string) string {
	t.Helper()
	re := regexp.MustCompile(`<` + regexp.QuoteMeta(name) + `>([^<]*)</` + regexp.QuoteMeta(name) + `>`)
	m := re.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("served bytes carry no <%s> element; body=%s", name, body)
	}
	return m[1]
}

// hexBinaryAttrText reads a hexBinary field served in ATTRIBUTE position, and
// additionally fails if the same name also appears as a child element. The
// second check is the one with teeth: a field the schema declares as an
// attribute but the Go type models as an element is IEEECORE-103, and it made
// the EPRI reference client abandon the whole DERControlList parse.
func hexBinaryAttrText(t *testing.T, body, name string) string {
	t.Helper()
	if strings.Contains(body, "<"+name+">") {
		t.Fatalf("served bytes carry <%s> as a child ELEMENT; the schema declares it as an attribute; body=%s", name, body)
	}
	re := regexp.MustCompile(regexp.QuoteMeta(name) + `="([^"]*)"`)
	m := re.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("served bytes carry no %s attribute; body=%s", name, body)
	}
	return m[1]
}

// assertHexBinaryText asserts that got is the expected hexBinary text AND
// that a hexBinary parser reading it recovers wantValue. The second check is
// the one that would have failed before this change: the text and the number
// have to agree under a HEX reading, not a decimal one.
func assertHexBinaryText(t *testing.T, field, got, want string, wantValue uint64) {
	t.Helper()
	if got != want {
		t.Errorf("%s served text = %q, want %q", field, got, want)
	}
	if len(got)%2 != 0 {
		t.Errorf("%s served text %q has an odd digit count; hexBinary requires whole octets (sep.xsd:6241) and the EPRI parser rejects an unpaired nibble (xml_parse.c:62)", field, got)
	}
	parsed, err := strconv.ParseUint(got, 16, 64)
	if err != nil {
		t.Fatalf("%s served text %q does not parse as hexBinary: %v", field, got, err)
	}
	if parsed != wantValue {
		t.Errorf("%s served text %q reads as %d under a hexBinary parser, want %d", field, got, parsed, wantValue)
	}
}

// hexBinaryDoc is one converted field: how to build a document carrying it,
// which element or attribute to read, and what the bytes must say.
//
// attr selects the XML position the field occupies. It is not cosmetic: the
// hexBinary family reaches the wire through MarshalXML in element position and
// through MarshalXMLAttr in attribute position, so the two positions exercise
// different code and a field checked in the wrong one is not checked at all.
type hexBinaryDoc struct {
	name    string
	elem    string
	value   uint64
	want    string
	attr    bool
	marshal func() (any, error)
}

// hexBinaryFieldCases covers EVERY field converted to the hexBinary family.
// Each entry produces a served-bytes assertion. Adding a hexBinary-derived
// field without adding a row here is how the next instance of this defect
// ships.
func hexBinaryFieldCases() []hexBinaryDoc {
	return []hexBinaryDoc{
		{
			name: "DERCapability.modesSupported", elem: "modesSupported", value: 2048, want: "0800",
			marshal: func() (any, error) {
				modes := sep2.DERControlType(2048)
				return &sep2.DERCapability{ModesSupported: &modes}, nil
			},
		},
		{
			name: "DERSettings.modesEnabled", elem: "modesEnabled", value: 2048, want: "0800",
			marshal: func() (any, error) {
				modes := sep2.DERControlType(2048)
				return &sep2.DERSettings{ModesEnabled: &modes}, nil
			},
		},
		{
			name: "DERStatus.alarmStatus", elem: "alarmStatus", value: 0x10, want: "10",
			marshal: func() (any, error) {
				alarm := sep2.HexBinary32(0x10)
				return &sep2.DERStatus{AlarmStatus: &alarm}, nil
			},
		},
		{
			name: "ConnectStatusType.value", elem: "value", value: 0x0B, want: "0B",
			marshal: func() (any, error) {
				return &sep2.DERStatus{GenConnectStatus: &sep2.ConnectStatusType{Value: 0x0B}}, nil
			},
		},
		{
			name: "DeviceInformation.functionsImplemented", elem: "functionsImplemented", value: 0x1234567890, want: "1234567890",
			marshal: func() (any, error) {
				funcs := sep2.HexBinary64(0x1234567890)
				return &sep2.DeviceInformation{FunctionsImplemented: &funcs}, nil
			},
		},
		{
			name: "EndDeviceControl.deviceCategory", elem: "deviceCategory", value: 0x40, want: "40",
			marshal: func() (any, error) {
				cat := sep2.DeviceCategoryType(0x40)
				return &sep2.EndDeviceControl{DeviceCategory: &cat}, nil
			},
		},
		{
			name: "UsagePoint.roleFlags", elem: "roleFlags", value: 9, want: "09",
			marshal: func() (any, error) {
				return &sep2.UsagePoint{RoleFlags: 9}, nil
			},
		},
		{
			name: "MirrorUsagePoint.roleFlags", elem: "roleFlags", value: 9, want: "09",
			marshal: func() (any, error) {
				return &sep2.MirrorUsagePoint{RoleFlags: 9}, nil
			},
		},
		{
			name: "Reading.qualityFlags", elem: "qualityFlags", value: 0, want: "00",
			marshal: func() (any, error) {
				quality := sep2.HexBinary16(0)
				return &sep2.Reading{QualityFlags: &quality}, nil
			},
		},
		{
			// ATTRIBUTE position: sep.xsd:5440 declares responseRequired as
			// an xs:attribute on RespondableResource. This row read the
			// ELEMENT position until IEEECORE-103, which is why it stayed
			// green while the served bytes were unparseable to the EPRI
			// reference client: it was asserting the hex text of a field
			// that should never have been an element in the first place.
			name: "DERControl.responseRequired", elem: "responseRequired", value: 3, want: "03", attr: true,
			marshal: func() (any, error) {
				mask := sep2.HexBinary8(3)
				ctrl := sep2.DERControl{}
				ctrl.ResponseRequired = &mask
				return &ctrl, nil
			},
		},
		{
			name: "DERControlResponse.modesResponded", elem: "modesResponded", value: 2048, want: "0800",
			marshal: func() (any, error) {
				modes := sep2.DERControlType(2048)
				return &sep2.DERControlResponse{ModesResponded: &modes}, nil
			},
		},
	}
}

// TestDERCapabilityModesSupported2048ServedAsHex is the reported live case,
// asserted on the exact served bytes. The advertised bitmap 2048 (0x800,
// bit 11) MUST appear as "0800". Before this change it appeared as "2048",
// which a hexBinary parser reads as 8264, so the client acted on a mode
// bitmap the server never advertised and neither side errored.
func TestDERCapabilityModesSupported2048ServedAsHex(t *testing.T) {
	t.Parallel()

	modes := sep2.DERControlType(2048)
	derCap := sep2.DERCapability{ModesSupported: &modes}

	data, err := xml.Marshal(&derCap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(data)

	if strings.Contains(body, "<modesSupported>2048</modesSupported>") {
		t.Fatalf("served bytes still carry the decimal form <modesSupported>2048</modesSupported>; a hexBinary parser reads that as %d, not 2048; body=%s",
			0x2048, body)
	}

	got := hexBinaryElementText(t, body, "modesSupported")
	assertHexBinaryText(t, "DERCapability.modesSupported", got, "0800", 2048)

	// Model the EPRI client's read explicitly: parse the served text as
	// hex and require the bit the server meant to advertise.
	parsed, err := strconv.ParseUint(got, 16, 32)
	if err != nil {
		t.Fatalf("hexBinary parse of %q: %v", got, err)
	}
	if parsed != 2048 {
		t.Errorf("hexBinary reading of served text = %d, want 2048", parsed)
	}
	if parsed == 8264 {
		t.Errorf("hexBinary reading of served text = 8264; this is the reported corruption")
	}

	// The numeric value in the struct is untouched by the encoding change.
	if *derCap.ModesSupported != 2048 {
		t.Errorf("ModesSupported = %d after marshal, want 2048; only the serialization was supposed to change", *derCap.ModesSupported)
	}
}

// TestHexBinaryFieldsServedBytes asserts the served bytes of every field
// converted to the hexBinary family.
func TestHexBinaryFieldsServedBytes(t *testing.T) {
	t.Parallel()

	for _, tc := range hexBinaryFieldCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			doc, err := tc.marshal()
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			data, err := xml.Marshal(doc)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var got string
			if tc.attr {
				got = hexBinaryAttrText(t, string(data), tc.elem)
			} else {
				got = hexBinaryElementText(t, string(data), tc.elem)
			}
			assertHexBinaryText(t, tc.name, got, tc.want, tc.value)
		})
	}
}

// TestHexBinaryFamilyElementServedBytes asserts the encoding rule itself at
// each declared width, including zero and the maximum the width can hold.
//
// Zero serves as "00" rather than an empty element or a full-width run of
// zeros: hexBinary needs at least one whole octet, and the maxLength facet
// counts octets, so a shorter-than-maximum value is conformant
// (sep.xsd:6257 documents HexBinary32 as "8 hex characters max"). The EPRI
// client right-aligns a short value into the wider field and zero-fills the
// front (xml_parse.c:65-68), so the number survives.
func TestHexBinaryFamilyElementServedBytes(t *testing.T) {
	t.Parallel()

	type wrap8 struct {
		XMLName xml.Name `xml:"w"`
		V       sep2.HexBinary8
	}
	type wrap16 struct {
		XMLName xml.Name `xml:"w"`
		V       sep2.HexBinary16
	}
	type wrap32 struct {
		XMLName xml.Name `xml:"w"`
		V       sep2.HexBinary32
	}
	type wrap64 struct {
		XMLName xml.Name `xml:"w"`
		V       sep2.HexBinary64
	}

	cases := []struct {
		name  string
		doc   any
		value uint64
		want  string
	}{
		{"hexBinary8_zero", &wrap8{V: 0}, 0, "00"},
		{"hexBinary8_low_nibble_pads", &wrap8{V: 9}, 9, "09"},
		{"hexBinary8_max", &wrap8{V: 0xFF}, 0xFF, "FF"},
		{"hexBinary16_zero", &wrap16{V: 0}, 0, "00"},
		{"hexBinary16_three_digits_pad", &wrap16{V: 0x12C}, 0x12C, "012C"},
		{"hexBinary16_max", &wrap16{V: 0xFFFF}, 0xFFFF, "FFFF"},
		{"hexBinary32_zero", &wrap32{V: 0}, 0, "00"},
		{"hexBinary32_live_2048", &wrap32{V: 2048}, 2048, "0800"},
		{"hexBinary32_max", &wrap32{V: 0xFFFFFFFF}, 0xFFFFFFFF, "FFFFFFFF"},
		{"hexBinary64_zero", &wrap64{V: 0}, 0, "00"},
		{"hexBinary64_max", &wrap64{V: 0xFFFFFFFFFFFFFFFF}, 0xFFFFFFFFFFFFFFFF, "FFFFFFFFFFFFFFFF"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data, err := xml.Marshal(tc.doc)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			got := hexBinaryElementText(t, string(data), "V")
			assertHexBinaryText(t, tc.name, got, tc.want, tc.value)
			if strings.ToUpper(got) != got {
				t.Errorf("%s served text %q is not uppercase; uppercase is the xs:hexBinary canonical lexical form", tc.name, got)
			}
		})
	}
}

// TestHexBinaryFamilyAttrServedBytes asserts the attribute encoding. sep.xsd
// puts responseRequired (HexBinary8) in attribute position on
// RespondableResource (sep.xsd:5440), and encoding/xml would render an
// attribute-tagged integer as decimal without these methods, which is the
// same corruption in a different position.
func TestHexBinaryFamilyAttrServedBytes(t *testing.T) {
	t.Parallel()

	type attrDoc struct {
		XMLName xml.Name         `xml:"w"`
		A8      sep2.HexBinary8  `xml:"a8,attr"`
		A16     sep2.HexBinary16 `xml:"a16,attr"`
		A32     sep2.HexBinary32 `xml:"a32,attr"`
		A64     sep2.HexBinary64 `xml:"a64,attr"`
	}

	data, err := xml.Marshal(&attrDoc{A8: 3, A16: 0x12C, A32: 2048, A64: 0})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(data)

	want := map[string]string{"a8": "03", "a16": "012C", "a32": "0800", "a64": "00"}
	for attr, wantText := range want {
		re := regexp.MustCompile(regexp.QuoteMeta(attr) + `="([^"]*)"`)
		m := re.FindStringSubmatch(body)
		if m == nil {
			t.Fatalf("served bytes carry no %s attribute; body=%s", attr, body)
		}
		if m[1] != wantText {
			t.Errorf("attribute %s served as %q, want %q; body=%s", attr, m[1], wantText, body)
		}
	}
}

// TestHexBinaryFieldsRoundTripFromWire asserts the UNMARSHAL direction on
// every converted field: hex text on the wire must decode back to the same
// number. A field that marshals as hex and parses as decimal corrupts
// client-supplied data on PUT and POST instead of server-supplied data on
// GET, which is the same defect pointed the other way.
func TestHexBinaryFieldsRoundTripFromWire(t *testing.T) {
	t.Parallel()

	t.Run("DERCapability.modesSupported", func(t *testing.T) {
		t.Parallel()
		var got sep2.DERCapability
		if err := xml.Unmarshal([]byte(`<DERCapability xmlns="urn:ieee:std:2030.5:ns"><modesSupported>0800</modesSupported></DERCapability>`), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.ModesSupported == nil || *got.ModesSupported != 2048 {
			t.Fatalf("ModesSupported = %v, want 2048", got.ModesSupported)
		}
	})

	t.Run("DERSettings.modesEnabled", func(t *testing.T) {
		t.Parallel()
		var got sep2.DERSettings
		if err := xml.Unmarshal([]byte(`<DERSettings xmlns="urn:ieee:std:2030.5:ns"><modesEnabled>0800</modesEnabled></DERSettings>`), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.ModesEnabled == nil || *got.ModesEnabled != 2048 {
			t.Fatalf("ModesEnabled = %v, want 2048", got.ModesEnabled)
		}
	})

	t.Run("DERStatus.alarmStatus_and_ConnectStatusType.value", func(t *testing.T) {
		t.Parallel()
		var got sep2.DERStatus
		if err := xml.Unmarshal([]byte(`<DERStatus xmlns="urn:ieee:std:2030.5:ns"><alarmStatus>10</alarmStatus><genConnectStatus><dateTime>1604963587</dateTime><value>0B</value></genConnectStatus></DERStatus>`), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.AlarmStatus == nil || *got.AlarmStatus != 0x10 {
			t.Fatalf("AlarmStatus = %v, want 0x10", got.AlarmStatus)
		}
		if got.GenConnectStatus == nil || got.GenConnectStatus.Value != 0x0B {
			t.Fatalf("GenConnectStatus.Value = %v, want 0x0B", got.GenConnectStatus)
		}
	})

	t.Run("DeviceInformation.functionsImplemented", func(t *testing.T) {
		t.Parallel()
		var got sep2.DeviceInformation
		if err := xml.Unmarshal([]byte(`<DeviceInformation xmlns="urn:ieee:std:2030.5:ns"><functionsImplemented>1234567890</functionsImplemented></DeviceInformation>`), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.FunctionsImplemented == nil || *got.FunctionsImplemented != 0x1234567890 {
			t.Fatalf("FunctionsImplemented = %v, want 0x1234567890", got.FunctionsImplemented)
		}
	})

	t.Run("EndDeviceControl.deviceCategory", func(t *testing.T) {
		t.Parallel()
		var got sep2.EndDeviceControl
		if err := xml.Unmarshal([]byte(`<EndDeviceControl xmlns="urn:ieee:std:2030.5:ns"><deviceCategory>40</deviceCategory></EndDeviceControl>`), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.DeviceCategory == nil || *got.DeviceCategory != 0x40 {
			t.Fatalf("DeviceCategory = %v, want 0x40", got.DeviceCategory)
		}
	})

	t.Run("UsagePoint.roleFlags", func(t *testing.T) {
		t.Parallel()
		var got sep2.UsagePoint
		if err := xml.Unmarshal([]byte(`<UsagePoint xmlns="urn:ieee:std:2030.5:ns"><roleFlags>09</roleFlags><serviceCategoryKind>0</serviceCategoryKind><status>1</status></UsagePoint>`), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.RoleFlags != 9 {
			t.Fatalf("RoleFlags = %d, want 9", got.RoleFlags)
		}
	})

	t.Run("MirrorUsagePoint.roleFlags", func(t *testing.T) {
		t.Parallel()
		var got sep2.MirrorUsagePoint
		if err := xml.Unmarshal([]byte(`<MirrorUsagePoint xmlns="urn:ieee:std:2030.5:ns"><roleFlags>09</roleFlags><serviceCategoryKind>0</serviceCategoryKind><status>1</status></MirrorUsagePoint>`), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.RoleFlags != 9 {
			t.Fatalf("RoleFlags = %d, want 9", got.RoleFlags)
		}
	})

	t.Run("Reading.qualityFlags", func(t *testing.T) {
		t.Parallel()
		var got sep2.Reading
		if err := xml.Unmarshal([]byte(`<Reading xmlns="urn:ieee:std:2030.5:ns"><qualityFlags>0080</qualityFlags></Reading>`), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.QualityFlags == nil || *got.QualityFlags != 0x80 {
			t.Fatalf("QualityFlags = %v, want 0x80", got.QualityFlags)
		}
	})

	// responseRequired arrives in ATTRIBUTE position (sep.xsd:5440), so the
	// inbound path exercises UnmarshalXMLAttr rather than UnmarshalXML. The
	// document below is the spec-conformant form a conforming peer sends;
	// before IEEECORE-103 this subtest fed the element form, so it was
	// asserting that we could read back our own non-conformant output.
	t.Run("DERControl.responseRequired", func(t *testing.T) {
		t.Parallel()
		var got sep2.DERControl
		if err := xml.Unmarshal([]byte(`<DERControl xmlns="urn:ieee:std:2030.5:ns" replyTo="/rsps/1/rsp" responseRequired="03"></DERControl>`), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.ResponseRequired == nil || *got.ResponseRequired != 3 {
			t.Fatalf("ResponseRequired = %v, want 3", got.ResponseRequired)
		}
		if got.ReplyTo != "/rsps/1/rsp" {
			t.Fatalf("ReplyTo = %q, want %q", got.ReplyTo, "/rsps/1/rsp")
		}
	})

	t.Run("DERControlResponse.modesResponded", func(t *testing.T) {
		t.Parallel()
		var got sep2.DERControlResponse
		if err := xml.Unmarshal([]byte(`<DERControlResponse xmlns="urn:ieee:std:2030.5:ns"><modesResponded>0800</modesResponded></DERControlResponse>`), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.ModesResponded == nil || *got.ModesResponded != 2048 {
			t.Fatalf("ModesResponded = %v, want 2048", got.ModesResponded)
		}
	})
}

// TestHexBinaryMarshalUnmarshalRoundTrip asserts marshal then unmarshal
// returns the original numeric value at every width, across the whole
// range each width can hold.
func TestHexBinaryMarshalUnmarshalRoundTrip(t *testing.T) {
	t.Parallel()

	type wrap32 struct {
		XMLName xml.Name `xml:"w"`
		V       sep2.HexBinary32
	}

	values := []uint64{0, 1, 9, 0x10, 0x12C, 2048, 8264, 0xFFFF, 0x01000000, 0xFFFFFFFF}
	for _, v := range values {
		v := v
		t.Run(strconv.FormatUint(v, 10), func(t *testing.T) {
			t.Parallel()
			data, err := xml.Marshal(&wrap32{V: sep2.HexBinary32(v)})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var got wrap32
			if err := xml.Unmarshal(data, &got); err != nil {
				t.Fatalf("unmarshal of %s: %v", data, err)
			}
			if uint64(got.V) != v {
				t.Errorf("round-trip of %d via %s = %d, want %d", v, data, got.V, v)
			}
		})
	}
}

// TestHexBinaryUnmarshalAcceptsPeerVariants documents the deliberate input
// leniency: a peer that sends lowercase, or omits the leading zero of an
// odd-digit value, has an unambiguous intent, and dropping the resource
// would turn its cosmetic defect into lost data. The EPRI client emits
// lowercase (hex_char, xml_output.c:46-49), so lowercase in particular is
// not hypothetical.
func TestHexBinaryUnmarshalAcceptsPeerVariants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		text string
		want sep2.HexBinary32
	}{
		{"0800", 2048},
		{"800", 2048},   // unpadded, not schema-conformant but unambiguous
		{"0800 ", 2048}, // trailing whitespace
		{"ffff", 0xFFFF},
		{"FFFF", 0xFFFF},
		{"00", 0},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(strings.TrimSpace(tc.text), func(t *testing.T) {
			t.Parallel()
			var got sep2.DERCapability
			doc := `<DERCapability xmlns="urn:ieee:std:2030.5:ns"><modesSupported>` + tc.text + `</modesSupported></DERCapability>`
			if err := xml.Unmarshal([]byte(doc), &got); err != nil {
				t.Fatalf("unmarshal %q: %v", tc.text, err)
			}
			if got.ModesSupported == nil || *got.ModesSupported != tc.want {
				t.Errorf("modesSupported %q decoded to %v, want %d", tc.text, got.ModesSupported, tc.want)
			}
		})
	}
}

// TestHexBinaryUnmarshalRejectsInvalidText asserts the errors are surfaced
// rather than swallowed into a wrong number. Anything whose numeric reading
// is not well defined has to fail loudly: silently substituting a value is
// exactly the failure mode this whole change exists to remove.
func TestHexBinaryUnmarshalRejectsInvalidText(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		text string
	}{
		{"empty", ""},
		{"whitespace_only", "   "},
		{"non_hex_characters", "80G0"},
		{"hex_prefix_not_permitted", "0x800"},
		{"overflows_declared_width", "1FFFFFFFF"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got sep2.DERCapability
			doc := `<DERCapability xmlns="urn:ieee:std:2030.5:ns"><modesSupported>` + tc.text + `</modesSupported></DERCapability>`
			if err := xml.Unmarshal([]byte(doc), &got); err == nil {
				t.Errorf("unmarshal of %q returned no error; decoded ModesSupported = %v", tc.text, got.ModesSupported)
			}
		})
	}
}

// TestHexBinaryAttrUnmarshalRoundTrip asserts the attribute direction parses
// back, matching the element direction.
func TestHexBinaryAttrUnmarshalRoundTrip(t *testing.T) {
	t.Parallel()

	type attrDoc struct {
		XMLName xml.Name         `xml:"w"`
		A8      sep2.HexBinary8  `xml:"a8,attr"`
		A16     sep2.HexBinary16 `xml:"a16,attr"`
		A32     sep2.HexBinary32 `xml:"a32,attr"`
		A64     sep2.HexBinary64 `xml:"a64,attr"`
	}

	var got attrDoc
	if err := xml.Unmarshal([]byte(`<w a8="03" a16="012C" a32="0800" a64="FFFFFFFFFFFFFFFF"></w>`), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.A8 != 3 {
		t.Errorf("a8 = %d, want 3", got.A8)
	}
	if got.A16 != 0x12C {
		t.Errorf("a16 = %d, want %d", got.A16, 0x12C)
	}
	if got.A32 != 2048 {
		t.Errorf("a32 = %d, want 2048", got.A32)
	}
	if got.A64 != 0xFFFFFFFFFFFFFFFF {
		t.Errorf("a64 = %d, want max", got.A64)
	}

	var bad attrDoc
	if err := xml.Unmarshal([]byte(`<w a8="ZZ"></w>`), &bad); err == nil {
		t.Errorf("unmarshal of a8=\"ZZ\" returned no error; a8 decoded to %d", bad.A8)
	}
}

// TestHexBinaryOmitEmptyUnchanged guards the pointer-field omitempty
// contract across the type change. A nil pointer still omits the element
// entirely, and a pointer to zero still serves "00" rather than vanishing:
// the numeric zero is a legitimate bitmap, not an absent field.
func TestHexBinaryOmitEmptyUnchanged(t *testing.T) {
	t.Parallel()

	data, err := xml.Marshal(&sep2.DERCapability{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "modesSupported") {
		t.Errorf("nil ModesSupported still served an element; body=%s", data)
	}

	zero := sep2.DERControlType(0)
	data, err = xml.Marshal(&sep2.DERCapability{ModesSupported: &zero})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := hexBinaryElementText(t, string(data), "modesSupported")
	assertHexBinaryText(t, "DERCapability.modesSupported zero", got, "00", 0)
}
