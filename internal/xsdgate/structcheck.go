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

// KindIntegerWidth means a Go integer field can hold a value the XSD integer
// built-in its element or attribute resolves to cannot: it is wider, signed
// over an unsigned type, or unsigned at the full width of a signed one. The
// marshaller would serialize such a value without complaint. See #111.
const KindIntegerWidth ProblemKind = "integer-width"

// KindIntegerUnresolved means a Go integer field is bound to an XSD type that
// resolves to no built-in this gate knows, so its range cannot be compared.
// Reporting it keeps the integer check from passing by skipping.
const KindIntegerUnresolved ProblemKind = "integer-unresolved"

// intWidth is the bit width and signedness of an integer type, Go or XSD.
type intWidth struct {
	bits   int
	signed bool
}

func (w intWidth) String() string {
	if w.signed {
		return fmt.Sprintf("%d-bit signed", w.bits)
	}
	return fmt.Sprintf("%d-bit unsigned", w.bits)
}

// fitsIn reports whether every value of w is representable in x.
func (w intWidth) fitsIn(x intWidth) bool {
	switch {
	case w.signed && !x.signed:
		return false
	case !w.signed && x.signed:
		return w.bits < x.bits
	}
	return w.bits <= x.bits
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

// goIntWidth measures t by its underlying Kind, so a named type over an
// integer (OneHourRange over int16) is judged by its storage.
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

// integerBase follows restriction bases and simpleContent extensions to the
// built-in a type's value bottoms out at. It returns "" for a dangling name,
// a cycle, element content, or a built-in outside isXSDBuiltin, and the chain
// of names it walked either way.
func (s *Schema) integerBase(typeName string) (builtin string, chain []string) {
	seen := map[string]bool{}
	for name := typeName; ; {
		chain = append(chain, name)
		if seen[name] {
			return "", chain
		}
		seen[name] = true
		if st, ok := s.simpleTypes[name]; ok {
			name = st.Base
			continue
		}
		if ct, ok := s.complexTypes[name]; ok && ct.SimpleContent {
			name = ct.Base
			continue
		}
		if isXSDBuiltin(name) {
			return name, chain
		}
		return "", chain
	}
}

// elementValueType strips pointers and, for a repeated element, the slice or
// array around it. encoding/xml repeats the element once per item, except for
// a byte slice or array, which it writes as character data.
func elementValueType(ft reflect.Type) reflect.Type {
	for ft.Kind() == reflect.Ptr {
		ft = ft.Elem()
	}
	if k := ft.Kind(); (k == reflect.Slice || k == reflect.Array) && ft.Elem().Kind() != reflect.Uint8 {
		ft = ft.Elem()
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
	}
	return ft
}

// checkIntegerWidth reports a Go integer field whose range its XSD type
// cannot hold, or whose XSD type cannot be resolved. A type resolving to a
// non-integer built-in such as hexBinary is left to the lexical check. path
// is the Go field path; nodePath is the element or attribute path it binds.
func (s *Schema) checkIntegerWidth(ps *Problems, path, nodePath, typeName string, f reflect.StructField, xsdType, owner string) {
	gw, ok := goIntWidth(elementValueType(f.Type))
	if !ok {
		return
	}
	builtin, chain := s.integerBase(xsdType)
	if builtin == "" {
		*ps = append(*ps, Problem{
			Kind: KindIntegerUnresolved,
			Path: path,
			Message: fmt.Sprintf("field %s is a %s Go integer but %q on %s (declared by %s) has type chain %s, "+
				"which reaches no XSD built-in this gate knows, so its range cannot be checked",
				f.Name, gw, nodePath, typeName, owner, strings.Join(chain, " -> ")),
		})
		return
	}
	xw, isInt := xsdIntWidths[builtin]
	if !isInt || gw.fitsIn(xw) {
		return
	}
	*ps = append(*ps, Problem{
		Kind: KindIntegerWidth,
		Path: path,
		Message: fmt.Sprintf("field %s is a %s Go integer but %q on %s (declared by %s) resolves %s, a %s XSD integer; "+
			"the Go type permits a value outside the schema's range and the marshaller would serialize it",
			f.Name, gw, nodePath, typeName, owner, strings.Join(chain, " -> "), xw),
	})
}

// descentKey is a Go struct checked against a schema type. A recursive pair
// is walked once, since a deeper copy repeats the same comparisons.
type descentKey struct {
	goType  reflect.Type
	xsdType string
}

// checkChildIntegers applies the integer check to the struct modelling a
// child element, at every depth, against the type the schema gives that
// element. An integer it cannot pair with a declaration is reported as
// unresolved, except a character-data integer in a child struct or a tagged
// embedded struct, both of which this check does not reach.
func (s *Schema) checkChildIntegers(ps *Problems, path, nodePath string, ft reflect.Type, xsdType string, active map[descentKey]bool) error {
	t := elementValueType(ft)
	if t.Kind() != reflect.Struct {
		return nil
	}
	key := descentKey{t, xsdType}
	if active[key] {
		return nil
	}
	if _, ok := s.complexTypes[xsdType]; !ok {
		if holdsInteger(t, map[reflect.Type]bool{}) {
			*ps = append(*ps, Problem{
				Kind: KindIntegerUnresolved,
				Path: path,
				Message: fmt.Sprintf("%s is a struct holding Go integers but %q has type %s, which is not a complexType, "+
					"so none of those integers can be paired with a declaration", path, nodePath, xsdType),
			})
		}
		return nil
	}
	elems, err := s.EffectiveElements(xsdType)
	if err != nil {
		return err
	}
	attrs, err := s.EffectiveAttributes(xsdType)
	if err != nil {
		return err
	}
	elemByName := make(map[string]Element, len(elems))
	for _, e := range elems {
		elemByName[e.Name] = e
	}
	attrByName := make(map[string]Attribute, len(attrs))
	for _, a := range attrs {
		attrByName[a.Name] = a
	}

	active[key] = true
	defer delete(active, key)
	for _, f := range flattenFields(t) {
		name, isAttr, _, skip := parseXMLTag(f)
		if skip {
			continue
		}
		fieldPath := path + "." + f.Name
		if isAttr {
			attrPath := nodePath + "/@" + name
			if a, ok := attrByName[name]; ok {
				s.checkIntegerWidth(ps, fieldPath, attrPath, xsdType, f, a.Type, a.Owner)
			} else {
				reportUndeclaredIntegers(ps, fieldPath, attrPath, xsdType, f)
			}
			continue
		}
		elemPath := nodePath + "/" + name
		el, ok := elemByName[name]
		if !ok {
			reportUndeclaredIntegers(ps, fieldPath, elemPath, xsdType, f)
			continue
		}
		s.checkIntegerWidth(ps, fieldPath, elemPath, xsdType, f, el.Type, el.Owner)
		if err := s.checkChildIntegers(ps, fieldPath, elemPath, f.Type, el.Type, active); err != nil {
			return err
		}
	}
	return nil
}

// reportUndeclaredIntegers reports a child field that carries integers but
// binds a name its schema type does not declare, since nothing gives a range.
func reportUndeclaredIntegers(ps *Problems, path, nodePath, typeName string, f reflect.StructField) {
	if !holdsInteger(elementValueType(f.Type), map[reflect.Type]bool{}) {
		return
	}
	*ps = append(*ps, Problem{
		Kind: KindIntegerUnresolved,
		Path: path,
		Message: fmt.Sprintf("field %s holds Go integers but %s declares nothing at %q, so their range cannot be checked",
			f.Name, typeName, nodePath),
	})
}

// holdsInteger reports whether t is a Go integer or a struct reaching one
// through its marshalled fields.
func holdsInteger(t reflect.Type, seen map[reflect.Type]bool) bool {
	if _, ok := goIntWidth(t); ok {
		return true
	}
	if t.Kind() != reflect.Struct || seen[t] {
		return false
	}
	seen[t] = true
	for _, f := range flattenFields(t) {
		if _, _, _, skip := parseXMLTag(f); !skip && holdsInteger(elementValueType(f.Type), seen) {
			return true
		}
	}
	return false
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
//   - Go integer fields (including pointers, slices and arrays of them) whose
//     range the XSD integer type cannot hold, or whose XSD type does not
//     resolve. This check alone descends into the structs modelling child
//     elements, at any depth, and reports by Go field path.
//
// Embedded structs are flattened in declaration position, matching how
// encoding/xml lays them out.
//
// It does NOT compare non-integer Go types with schema types, nor integer
// fields with facets such as minInclusive, so a field bound to the right
// element name with an unrepresentable Go type can pass here. Marshalling a
// value and running Validate catches the lexical consequence.
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
	active := map[descentKey]bool{{t, typeName}: true}

	for _, f := range flattenFields(t) {
		name, isAttr, omitempty, skip := parseXMLTag(f)
		if skip {
			continue
		}

		if isAttr {
			if a, ok := attrByName[name]; ok {
				s.checkIntegerWidth(&ps, typeName+"."+f.Name, typeName+"/@"+name, typeName, f, a.Type, a.Owner)
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

		s.checkIntegerWidth(&ps, typeName+"."+f.Name, typeName+"/"+name, typeName, f, el.Type, el.Owner)
		if err := s.checkChildIntegers(&ps, typeName+"."+f.Name, typeName+"/"+name, f.Type, el.Type, active); err != nil {
			return nil, err
		}

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
