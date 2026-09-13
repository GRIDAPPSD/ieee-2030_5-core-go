package xsdgate

import (
	"reflect"
	"strings"
	"testing"
)

// integerFixtureXSD is a synthetic schema, not sep.xsd, so the integer checks
// in CheckStruct are exercised on every run rather than only when an
// operator-supplied schema is present.
const integerFixtureXSD = `<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns="urn:example:fixture" targetNamespace="urn:example:fixture">
  <xs:simpleType name="Int8"><xs:restriction base="xs:byte"/></xs:simpleType>
  <xs:simpleType name="Int16"><xs:restriction base="xs:short"/></xs:simpleType>
  <xs:simpleType name="Int32"><xs:restriction base="xs:int"/></xs:simpleType>
  <xs:simpleType name="UInt8"><xs:restriction base="xs:unsignedByte"/></xs:simpleType>
  <xs:simpleType name="UInt32"><xs:restriction base="xs:unsignedInt"/></xs:simpleType>
  <xs:simpleType name="Ranged16">
    <xs:restriction base="Int16">
      <xs:minInclusive value="-10"/>
      <xs:maxInclusive value="10"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="Dangling"><xs:restriction base="NoSuchType"/></xs:simpleType>
  <xs:simpleType name="CycleA"><xs:restriction base="CycleB"/></xs:simpleType>
  <xs:simpleType name="CycleB"><xs:restriction base="CycleA"/></xs:simpleType>
  <xs:simpleType name="Unbounded"><xs:restriction base="xs:integer"/></xs:simpleType>
  <xs:simpleType name="Hex8">
    <xs:restriction base="xs:hexBinary"><xs:maxLength value="1"/></xs:restriction>
  </xs:simpleType>
  <xs:complexType name="Wrapped16">
    <xs:simpleContent><xs:extension base="Int16"/></xs:simpleContent>
  </xs:complexType>
  <xs:complexType name="Holder">
    <xs:sequence><xs:element name="inner" type="UInt32"/></xs:sequence>
  </xs:complexType>
</xs:schema>`

type fixtureRange int16

// integerHost builds a one-field struct type and a matching schema complex
// type, so each case isolates one Go type against one XSD type.
func integerHost(t *testing.T, goType reflect.Type, xsdType string, attr bool) (*Schema, string, reflect.Type) {
	t.Helper()
	var decl, tag string
	if attr {
		decl = `<xs:complexType name="Host"><xs:attribute name="v" type="` + xsdType + `"/></xs:complexType>`
		tag = `xml:"v,attr"`
	} else {
		decl = `<xs:complexType name="Host"><xs:sequence><xs:element name="v" type="` + xsdType +
			`" minOccurs="0" maxOccurs="unbounded"/></xs:sequence></xs:complexType>`
		tag = `xml:"v,omitempty"`
	}
	doc := strings.Replace(integerFixtureXSD, "</xs:schema>", decl+"</xs:schema>", 1)
	s, err := ParseSchema([]byte(doc))
	if err != nil {
		t.Fatalf("parse fixture schema: %v", err)
	}
	st := reflect.StructOf([]reflect.StructField{{Name: "V", Type: goType, Tag: reflect.StructTag(tag)}})
	return s, "Host", st
}

func integerProblems(ps Problems) []string {
	var out []string
	for _, line := range ps.Summary() {
		if strings.HasPrefix(line, string(KindIntegerWidth)) || strings.HasPrefix(line, string(KindIntegerUnresolved)) {
			out = append(out, line)
		}
	}
	return out
}

func TestCheckStructIntegerRange(t *testing.T) {
	t.Parallel()
	width := []string{"integer-width Host.V"}
	unresolved := []string{"integer-unresolved Host.V"}

	cases := []struct {
		name    string
		goType  reflect.Type
		xsdType string
		attr    bool
		want    []string
	}{
		{"int32 over xs:short is wider", reflect.TypeOf(int32(0)), "Int16", false, width},
		{"int16 over xs:short is equal", reflect.TypeOf(int16(0)), "Int16", false, nil},
		{"int8 over xs:short is narrower", reflect.TypeOf(int8(0)), "Int16", false, nil},
		{"pointer int32 through a two-level chain", reflect.TypeOf(new(int32)), "Ranged16", false, width},
		{"int over xs:int is wider", reflect.TypeOf(int(0)), "Int32", false, width},
		{"int64 over xs:unsignedInt", reflect.TypeOf(int64(0)), "UInt32", false, width},
		{"uint32 over xs:unsignedInt is equal", reflect.TypeOf(uint32(0)), "UInt32", false, nil},
		{"uint16 over xs:unsignedInt fits", reflect.TypeOf(uint16(0)), "UInt32", false, nil},
		{"uint16 over xs:short exceeds the signed maximum", reflect.TypeOf(uint16(0)), "Int16", false, width},
		{"uint8 over xs:short fits", reflect.TypeOf(uint8(0)), "Int16", false, nil},
		{"int8 over xs:unsignedByte permits negatives", reflect.TypeOf(int8(0)), "UInt8", false, width},
		{"uint32 over xs:int exceeds the signed maximum", reflect.TypeOf(uint32(0)), "Int32", false, width},
		{"slice of int64 over repeated xs:unsignedInt", reflect.TypeOf([]int64(nil)), "UInt32", false, width},
		{"slice of uint32 over repeated xs:unsignedInt", reflect.TypeOf([]uint32(nil)), "UInt32", false, nil},
		{"slice of pointer int32 over repeated xs:short", reflect.TypeOf([]*int32(nil)), "Int16", false, width},
		{"array of int32 over repeated xs:short", reflect.TypeOf([3]int32{}), "Int16", false, width},
		{"byte slice is character data, not repeated integers", reflect.TypeOf([]byte(nil)), "Hex8", false, nil},
		{"byte slice over an integer type is character data", reflect.TypeOf([]byte(nil)), "Int8", false, nil},
		{"byte array over an integer type is character data", reflect.TypeOf([4]byte{}), "Int8", false, nil},
		{"int32 over simpleContent extending xs:short", reflect.TypeOf(int32(0)), "Wrapped16", false, width},
		{"int16 over simpleContent extending xs:short", reflect.TypeOf(int16(0)), "Wrapped16", false, nil},
		{"named int16 type over xs:short", reflect.TypeOf(fixtureRange(0)), "Int16", false, nil},
		{"uint8 over hexBinary is left to the lexical check", reflect.TypeOf(uint8(0)), "Hex8", false, nil},
		{"string over xs:short is not an integer field", reflect.TypeOf(""), "Int16", false, nil},
		{"int16 over a facet-narrowed xs:short passes on width alone", reflect.TypeOf(int16(0)), "Ranged16", false, nil},
		{"dangling restriction base", reflect.TypeOf(int32(0)), "Dangling", false, unresolved},
		{"undeclared type", reflect.TypeOf(int32(0)), "NotDeclared", false, unresolved},
		{"restriction cycle", reflect.TypeOf(int32(0)), "CycleA", false, unresolved},
		{"built-in outside the gate's integer set", reflect.TypeOf(int64(0)), "Unbounded", false, unresolved},
		{"complex type with element content", reflect.TypeOf(uint16(0)), "Holder", false, unresolved},
		{"attribute int32 over xs:short is wider", reflect.TypeOf(int32(0)), "Int16", true, width},
		{"attribute int16 over xs:short is equal", reflect.TypeOf(int16(0)), "Int16", true, nil},
		{"attribute uint16 over xs:short exceeds the signed maximum", reflect.TypeOf(uint16(0)), "Int16", true, width},
		{"attribute dangling restriction base", reflect.TypeOf(int32(0)), "Dangling", true, unresolved},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, typeName, st := integerHost(t, tc.goType, tc.xsdType, tc.attr)
			ps, err := s.CheckStruct(typeName, st)
			if err != nil {
				t.Fatalf("CheckStruct: %v", err)
			}
			got := integerProblems(ps)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("integer problems = %v, want %v (all problems: %v)", got, tc.want, ps.Summary())
			}
		})
	}
}

// TestCheckStructIntegerMessages pins that a report names the resolved chain,
// which is what lets a CI log explain a width finding without the schema.
func TestCheckStructIntegerMessages(t *testing.T) {
	t.Parallel()
	s, typeName, st := integerHost(t, reflect.TypeOf(new(int32)), "Ranged16", false)
	ps, err := s.CheckStruct(typeName, st)
	if err != nil {
		t.Fatalf("CheckStruct: %v", err)
	}
	if len(ps) != 1 {
		t.Fatalf("problems = %v, want exactly one", ps.Summary())
	}
	for _, want := range []string{"32-bit signed", "Ranged16 -> Int16 -> short", "16-bit signed"} {
		if !strings.Contains(ps[0].Message, want) {
			t.Errorf("message %q does not contain %q", ps[0].Message, want)
		}
	}

	s, typeName, st = integerHost(t, reflect.TypeOf(int32(0)), "Dangling", false)
	if ps, err = s.CheckStruct(typeName, st); err != nil {
		t.Fatalf("CheckStruct: %v", err)
	}
	if len(ps) != 1 || !strings.Contains(ps[0].Message, "Dangling -> NoSuchType") {
		t.Errorf("unresolved problems = %v, want one naming the chain Dangling -> NoSuchType", ps)
	}
}

func TestIntegerBaseChain(t *testing.T) {
	t.Parallel()
	s, err := ParseSchema([]byte(integerFixtureXSD))
	if err != nil {
		t.Fatalf("parse fixture schema: %v", err)
	}
	cases := []struct {
		typeName    string
		wantBuiltin string
		wantChain   string
	}{
		{"Ranged16", "short", "Ranged16 -> Int16 -> short"},
		{"Wrapped16", "short", "Wrapped16 -> Int16 -> short"},
		{"Hex8", "hexBinary", "Hex8 -> hexBinary"},
		{"Dangling", "", "Dangling -> NoSuchType"},
		{"Holder", "", "Holder"},
		{"CycleA", "", "CycleA -> CycleB -> CycleA"},
		{"Unbounded", "", "Unbounded -> integer"},
	}
	for _, tc := range cases {
		builtin, chain := s.integerBase(tc.typeName)
		if builtin != tc.wantBuiltin || strings.Join(chain, " -> ") != tc.wantChain {
			t.Errorf("integerBase(%q) = %q, %q; want %q, %q",
				tc.typeName, builtin, strings.Join(chain, " -> "), tc.wantBuiltin, tc.wantChain)
		}
	}
}

// TestSchemaGateOneHourRangeResolvesToShort checks #111's premise against the
// real schema: both randomize elements resolve to xs:short. A failure prints
// the chain the gate walked, so a CI log explains it.
func TestSchemaGateOneHourRangeResolvesToShort(t *testing.T) {
	s := MustLoad(t)
	elems, err := s.EffectiveElements("RandomizableEvent")
	if err != nil {
		t.Fatalf("RandomizableEvent: %v", err)
	}
	found := 0
	for _, e := range elems {
		if e.Name != "randomizeDuration" && e.Name != "randomizeStart" {
			continue
		}
		found++
		if builtin, chain := s.integerBase(e.Type); builtin != "short" {
			t.Errorf("RandomizableEvent/%s resolves to %q via %s, want short", e.Name, builtin, strings.Join(chain, " -> "))
		}
	}
	if found != 2 {
		t.Errorf("RandomizableEvent declares %d randomize elements, want 2", found)
	}
}
