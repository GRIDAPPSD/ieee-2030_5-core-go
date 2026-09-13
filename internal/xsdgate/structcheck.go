package xsdgate

import (
	"fmt"
	"reflect"
	"strings"
)

// KindOmitemptyRequired means a struct field carrying a required schema
// element is tagged omitempty, so a zero value silently drops an element the
// standard mandates.
const KindOmitemptyRequired ProblemKind = "omitempty-required"

// KindStructOrder means struct field declaration order disagrees with the
// xsd:sequence. encoding/xml emits fields in declaration order, so this is an
// order defect that only becomes visible on the wire once both fields happen
// to be populated.
const KindStructOrder ProblemKind = "struct-order"

// KindIntegerWidth means a field's Go integer storage is WIDER than the XSD
// numeric built-in its element or attribute bottoms out at. The Go type then
// permits constructing a value the schema forbids, and the marshaller
// serializes it without complaint: this is the storage-capacity counterpart
// to KindLexical in validate.go, which catches an actual out-of-range VALUE
// but only once one has been set. See #111.
const KindIntegerWidth ProblemKind = "integer-width"

// intWidth is the bit width and signedness of an integer type, Go or XSD.
type intWidth struct {
	bits   int
	signed bool
}

// xsdIntWidths gives the width of every XSD built-in integer type this gate
// understands; it is the same name set as xsdIntRanges in lexical.go.
var xsdIntWidths = map[string]intWidth{
	"byte":          {8, true},
	"short":         {16, true},
	"int":           {32, true},
	"long":          {64, true},
	"unsignedByte":  {8, false},
	"unsignedShort": {16, false},
	"unsignedInt":   {32, false},
	"unsignedLong":  {64, false},
}

// goIntWidth reports t's bit width when t's underlying Kind is a plain
// integer, so a named type over an integer (OneHourRange over int16,
// HexBinary8 over uint8) is measured by its storage, not its declared name.
// ok is false for anything else (string, float, struct, and so on), which is
// how this check stays silent on non-numeric elements without a separate
// exclusion list.
func goIntWidth(t reflect.Type) (w intWidth, ok bool) {
	switch t.Kind() {
	case reflect.Int8:
		return intWidth{8, true}, true
	case reflect.Int16:
		return intWidth{16, true}, true
	case reflect.Int32:
		return intWidth{32, true}, true
	case reflect.Int64, reflect.Int:
		return intWidth{64, true}, true
	case reflect.Uint8:
		return intWidth{8, false}, true
	case reflect.Uint16:
		return intWidth{16, false}, true
	case reflect.Uint32:
		return intWidth{32, false}, true
	case reflect.Uint64, reflect.Uint:
		return intWidth{64, false}, true
	}
	return intWidth{}, false
}

// checkIntegerWidth appends a KindIntegerWidth problem when f's Go storage is
// wider than xsdType's resolved built-in. It is a no-op for a field whose Go
// kind is not a plain integer, or whose XSD type does not bottom out at one
// of the eight XSD integer built-ins (a hexBinary-, string-, or
// decimal-typed field, for instance): both cases fall through with ok=false.
func (s *Schema) checkIntegerWidth(ps *Problems, typeName string, f reflect.StructField, elementName, xsdType, owner string) {
	ft := f.Type
	for ft.Kind() == reflect.Ptr {
		ft = ft.Elem()
	}
	gw, ok := goIntWidth(ft)
	if !ok {
		return
	}
	builtin, _ := s.resolveBuiltin(xsdType)
	xw, ok := xsdIntWidths[builtin]
	if !ok {
		return
	}
	if gw.bits > xw.bits {
		*ps = append(*ps, Problem{
			Kind: KindIntegerWidth,
			Path: typeName + "." + f.Name,
			Message: fmt.Sprintf("field %s is a %d-bit Go integer but %q is xs:%s (%d-bit) on %s (declared by %s); "+
				"the Go type permits a value outside the schema's range and the marshaller would serialize it",
				f.Name, gw.bits, elementName, builtin, xw.bits, typeName, owner),
		})
	}
}

// CheckStruct validates a Go struct TYPE against a schema type, statically.
//
// This complements Validate rather than duplicating it. Validating marshalled
// output can only see fields that happened to be populated, so an ordering
// defect between two optional fields stays invisible until some future caller
// sets both. Checking the struct definition sees every field every time,
// which is what makes this the check that actually prevents the defect class
// rather than catching it later.
//
// It reports:
//
//   - Field declaration order against the effective xsd:sequence.
//   - omitempty on a field bound to a minOccurs>=1 element. This is the exact
//     shape of the DERCapability, DERSettings, DERStatus and UsagePoint
//     defects: the element is modelled correctly and then silently dropped.
//   - Fields tagged ",attr" that the schema declares as elements, and fields
//     tagged as elements that the schema declares as attributes.
//   - Field names absent from the schema entirely.
//
// Embedded structs are flattened in declaration position, matching how
// encoding/xml lays them out.
//
// It does NOT check Go types against schema types beyond placement, so a
// field bound to the right element name with an unrepresentable Go type is
// not reported here. Marshalling that value and running Validate catches the
// lexical consequence.
func (s *Schema) CheckStruct(typeName string, t reflect.Type) (Problems, error) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("xsdgate: CheckStruct needs a struct type, got %s", t.Kind())
	}
	if _, ok := s.complexTypes[typeName]; !ok {
		return nil, fmt.Errorf("xsdgate: %q is not a complexType in the schema", typeName)
	}

	elems, err := s.EffectiveElements(typeName)
	if err != nil {
		return nil, err
	}
	attrs, err := s.EffectiveAttributes(typeName)
	if err != nil {
		return nil, err
	}

	elemIndex := make(map[string]int, len(elems))
	elemByName := make(map[string]Element, len(elems))
	for i, e := range elems {
		elemIndex[e.Name] = i
		elemByName[e.Name] = e
	}
	attrByName := make(map[string]Attribute, len(attrs))
	for _, a := range attrs {
		attrByName[a.Name] = a
	}

	var ps Problems
	lastIdx, lastName := -1, ""

	for _, f := range flattenFields(t) {
		name, isAttr, omitempty, skip := parseXMLTag(f)
		if skip {
			continue
		}

		if isAttr {
			if a, ok := attrByName[name]; ok {
				s.checkIntegerWidth(&ps, typeName, f, name, a.Type, a.Owner)
				continue
			}
			if el, ok := elemByName[name]; ok {
				ps = append(ps, Problem{
					Kind: KindPlacement,
					Path: typeName + "." + f.Name,
					Message: fmt.Sprintf("field %s is tagged \",attr\" but the schema declares %q on %s as a CHILD ELEMENT (type %s, declared by %s)",
						f.Name, name, typeName, el.Type, el.Owner),
				})
				continue
			}
			ps = append(ps, Problem{
				Kind:    KindUnknownAttribute,
				Path:    typeName + "." + f.Name,
				Message: fmt.Sprintf("field %s is tagged \",attr\" as %q, which the schema does not declare on %s", f.Name, name, typeName),
			})
			continue
		}

		el, ok := elemByName[name]
		if !ok {
			if a, isSchemaAttr := attrByName[name]; isSchemaAttr {
				ps = append(ps, Problem{
					Kind: KindPlacement,
					Path: typeName + "." + f.Name,
					Message: fmt.Sprintf("field %s is tagged as a child element but the schema declares %q on %s as an ATTRIBUTE (type %s, declared by %s)",
						f.Name, name, typeName, a.Type, a.Owner),
				})
				continue
			}
			ps = append(ps, Problem{
				Kind:    KindUnknownElement,
				Path:    typeName + "." + f.Name,
				Message: fmt.Sprintf("field %s maps to element %q, which the schema does not declare on %s", f.Name, name, typeName),
			})
			continue
		}

		if el.Required && omitempty {
			ps = append(ps, Problem{
				Kind: KindOmitemptyRequired,
				Path: typeName + "." + f.Name,
				Message: fmt.Sprintf("field %s maps to %q which is minOccurs=\"1\" on %s (declared by %s), but is tagged omitempty; "+
					"a zero value silently drops an element the standard requires",
					f.Name, name, typeName, el.Owner),
			})
		}

		s.checkIntegerWidth(&ps, typeName, f, name, el.Type, el.Owner)

		idx := elemIndex[name]
		if idx < lastIdx {
			ps = append(ps, Problem{
				Kind: KindStructOrder,
				Path: typeName + "." + f.Name,
				Message: fmt.Sprintf("field %s maps to %q at sequence position %d, declared after %q at position %d; "+
					"encoding/xml emits fields in declaration order, so this produces out-of-sequence XML. Schema order on %s is: %s",
					f.Name, name, idx, lastName, lastIdx, typeName, sequenceSummary(elems)),
			})
		}
		if idx > lastIdx {
			lastIdx, lastName = idx, name
		}
	}
	return ps, nil
}

// flattenFields returns the struct's fields with embedded structs expanded in
// declaration position, mirroring encoding/xml's layout. An embedded struct
// that itself carries an XMLName is treated as a plain field, as encoding/xml
// does.
func flattenFields(t reflect.Type) []reflect.StructField {
	var out []reflect.StructField
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous {
			ft := f.Type
			for ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct && f.Tag.Get("xml") == "" {
				out = append(out, flattenFields(ft)...)
				continue
			}
		}
		if f.PkgPath != "" {
			// Unexported fields are never marshalled.
			continue
		}
		out = append(out, f)
	}
	return out
}

// parseXMLTag extracts the marshalled name and modifiers from a field's xml
// struct tag, applying encoding/xml's defaulting rules.
func parseXMLTag(f reflect.StructField) (name string, isAttr, omitempty, skip bool) {
	if f.Name == "XMLName" {
		return "", false, false, true
	}
	tag := f.Tag.Get("xml")
	if tag == "-" {
		return "", false, false, true
	}

	parts := strings.Split(tag, ",")
	name = parts[0]
	for _, opt := range parts[1:] {
		switch opt {
		case "attr":
			isAttr = true
		case "omitempty":
			omitempty = true
		case "chardata", "innerxml", "comment", "cdata", "any":
			// Not element or attribute bindings; nothing to check.
			skip = true
		}
	}
	if skip {
		return "", false, false, true
	}
	// A namespace-qualified tag name is "space local"; keep the local part.
	if i := strings.LastIndexByte(name, ' '); i >= 0 {
		name = name[i+1:]
	}
	if name == "" {
		// encoding/xml falls back to the Go field name.
		name = f.Name
	}
	return name, isAttr, omitempty, false
}
