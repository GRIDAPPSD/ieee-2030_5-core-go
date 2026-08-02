// Package xsdgate validates marshalled IEEE 2030.5 XML against the
// normative schema (see ../../schema).
//
// The schema is not distributed with this project. It is read at test time
// from a copy the operator supplies, and the gated tests skip when none is
// available. See the NOTICE file at the repository root.
//
// # Why this exists
//
// Round-tripping a value through encoding/xml proves almost nothing about
// wire correctness. Unmarshalling is order-insensitive, so a struct whose
// fields are declared out of xsd:sequence order round-trips cleanly and is
// still rejected by a strict peer. Missing required elements decode to zero
// values. Unknown elements are ignored outright. A value serialized in the
// wrong lexical form (decimal where the schema says hexBinary) round-trips
// perfectly and means a different number to the peer.
//
// Every one of those defects has actually shipped in this module, and every
// one was found by a human reading the XSD against struct tags. This package
// exists so the build finds them instead.
//
// # What this package checks
//
// Given a type name from the schema and a marshalled document, Validate
// reports:
//
//   - Element ORDER against the effective xsd:sequence, including elements
//     inherited through xsd:extension (base particles precede derived ones).
//   - PRESENCE of every minOccurs>=1 element.
//   - Repetition: an element with maxOccurs="1" appearing more than once.
//   - UNKNOWN elements, i.e. elements the schema does not declare on the type.
//   - Attribute-versus-element PLACEMENT, in both directions. A schema
//     attribute emitted as a child element, or a schema element emitted as an
//     attribute, is reported as a placement error rather than as a generic
//     unknown-name error, because that is the defect's actual shape.
//   - UNKNOWN attributes.
//   - PRESENCE of every use="required" attribute.
//   - Simple-type LEXICAL FORM for the built-in bases the schema restricts:
//     hexBinary (with maxLength), the signed and unsigned integer families
//     (with range), boolean, anyURI, and string. hexBinary is checked
//     strictly, since decimal-where-hex is silent and consequential.
//   - The root element's namespace, which must be the schema's
//     targetNamespace.
//
// Checking recurses into child elements using their declared types, so a
// defect nested three levels down is still caught.
//
// # What this package does NOT check
//
// This is deliberately not a general XSD validator. Do not mistake a passing
// Validate for full schema validation. It does not implement:
//
//   - xsd:choice, xsd:all, xsd:group, xsd:attributeGroup, substitution
//     groups, xsd:any wildcards, abstract types, or xsi:type redefinition.
//     None of these appear anywhere in sep.xsd, which is why their absence
//     is safe here and would not be safe against an arbitrary schema.
//     ParseSchema returns an error if it ever encounters one, so this
//     assumption cannot rot silently if the schema is revised.
//   - xsd:import and xsd:include. sep.xsd is self-contained.
//   - Identity constraints: xsd:key, xsd:keyref, xsd:unique.
//   - Facets beyond maxLength on hexBinary and the numeric ranges implied by
//     the integer base types. xsd:pattern, xsd:enumeration, minLength,
//     totalDigits and friends are parsed but NOT enforced. An out-of-
//     enumeration value passes.
//   - maxOccurs upper bounds other than the 1-versus-many distinction. An
//     element declared maxOccurs="5" is treated as unbounded.
//   - minOccurs on repeated elements beyond the presence check, so
//     minOccurs="2" is enforced only as "at least one".
//   - Mixed content, xsi:nil, and default or fixed value application.
//   - Namespace prefixing of descendants. The root namespace is checked;
//     descendants are matched on local name.
//
// The subset is chosen to cover the defect classes this module has actually
// produced, at zero dependency cost. It is not a conformance oracle.
package xsdgate

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ---------- raw XSD binding ----------
//
// These types mirror only the XSD constructs sep.xsd actually uses. Anything
// outside the subset is detected and rejected in ParseSchema rather than
// being silently ignored.

type xsdSchemaDoc struct {
	XMLName         xml.Name         `xml:"http://www.w3.org/2001/XMLSchema schema"`
	TargetNamespace string           `xml:"targetNamespace,attr"`
	Version         string           `xml:"version,attr"`
	ComplexTypes    []xsdComplexType `xml:"complexType"`
	SimpleTypes     []xsdSimpleType  `xml:"simpleType"`
	Elements        []xsdElementDecl `xml:"element"`
	Imports         []struct{}       `xml:"import"`
	Includes        []struct{}       `xml:"include"`
	Groups          []struct{}       `xml:"group"`
	AttributeGroups []struct{}       `xml:"attributeGroup"`
}

type xsdComplexType struct {
	Name           string         `xml:"name,attr"`
	Abstract       string         `xml:"abstract,attr"`
	Sequence       *xsdSequence   `xml:"sequence"`
	Choice         *struct{}      `xml:"choice"`
	All            *struct{}      `xml:"all"`
	GroupRef       *struct{}      `xml:"group"`
	Attributes     []xsdAttribute `xml:"attribute"`
	AttrGroupRefs  []struct{}     `xml:"attributeGroup"`
	ComplexContent *xsdDerivation `xml:"complexContent"`
	SimpleContent  *xsdDerivation `xml:"simpleContent"`
}

type xsdDerivation struct {
	Extension   *xsdExtension `xml:"extension"`
	Restriction *struct{}     `xml:"restriction"`
}

type xsdExtension struct {
	Base          string         `xml:"base,attr"`
	Sequence      *xsdSequence   `xml:"sequence"`
	Choice        *struct{}      `xml:"choice"`
	All           *struct{}      `xml:"all"`
	GroupRef      *struct{}      `xml:"group"`
	Attributes    []xsdAttribute `xml:"attribute"`
	AttrGroupRefs []struct{}     `xml:"attributeGroup"`
}

type xsdSequence struct {
	Elements []xsdElementDecl `xml:"element"`
	Choice   *struct{}        `xml:"choice"`
	All      *struct{}        `xml:"all"`
	GroupRef *struct{}        `xml:"group"`
	Any      *struct{}        `xml:"any"`
	Nested   []xsdSequence    `xml:"sequence"`
}

type xsdElementDecl struct {
	Name        string          `xml:"name,attr"`
	Ref         string          `xml:"ref,attr"`
	Type        string          `xml:"type,attr"`
	MinOccurs   string          `xml:"minOccurs,attr"`
	MaxOccurs   string          `xml:"maxOccurs,attr"`
	Nillable    string          `xml:"nillable,attr"`
	ComplexType *xsdComplexType `xml:"complexType"`
	SimpleType  *xsdSimpleType  `xml:"simpleType"`
}

type xsdAttribute struct {
	Name string `xml:"name,attr"`
	Ref  string `xml:"ref,attr"`
	Type string `xml:"type,attr"`
	Use  string `xml:"use,attr"`
}

type xsdSimpleType struct {
	Name        string          `xml:"name,attr"`
	Restriction *xsdRestriction `xml:"restriction"`
	Union       *struct{}       `xml:"union"`
	List        *struct{}       `xml:"list"`
}

type xsdRestriction struct {
	Base         string     `xml:"base,attr"`
	MaxLength    *xsdFacet  `xml:"maxLength"`
	MinLength    *xsdFacet  `xml:"minLength"`
	MaxInclusive *xsdFacet  `xml:"maxInclusive"`
	MinInclusive *xsdFacet  `xml:"minInclusive"`
	Enumerations []xsdFacet `xml:"enumeration"`
	Patterns     []xsdFacet `xml:"pattern"`
}

type xsdFacet struct {
	Value string `xml:"value,attr"`
}

// ---------- resolved model ----------

// Element is one particle of a complex type's effective sequence.
type Element struct {
	Name string
	Type string
	// Required reports minOccurs >= 1.
	Required bool
	// Repeatable reports maxOccurs != "1".
	Repeatable bool
	// Owner is the type that declared this particle, which may be an
	// ancestor of the type being validated. Reported in errors so an
	// inherited-particle defect points at the right definition.
	Owner string
}

// Attribute is one attribute of a complex type.
type Attribute struct {
	Name     string
	Type     string
	Required bool
	Owner    string
}

// ComplexType is a resolved complex type definition.
type ComplexType struct {
	Name string
	// Base is the xsd:extension base type name, or "" if the type derives
	// from nothing.
	Base string
	// SimpleContent reports that the type's content is a simple value
	// rather than child elements.
	SimpleContent bool
	// ownElements and ownAttributes exclude anything inherited.
	ownElements   []Element
	ownAttributes []Attribute
}

// SimpleType is a resolved simple type definition.
type SimpleType struct {
	Name string
	// Base is the immediate restriction base, which may itself be a
	// schema-defined simple type.
	Base string
	// MaxLength is the maxLength facet, or -1 when absent. For a hexBinary
	// restriction this is a length in BYTES, not hex digits.
	MaxLength int
}

// Schema is a parsed, resolved sep.xsd.
type Schema struct {
	TargetNamespace string
	Version         string

	complexTypes map[string]*ComplexType
	simpleTypes  map[string]*SimpleType
	// elements maps a top-level element name to its type name.
	elements map[string]string

	// memoised effective views
	effElements map[string][]Element
	effAttrs    map[string][]Attribute
}

// ParseSchema parses an XSD document into a resolved Schema.
//
// It returns an error if the document uses an XSD construct outside the
// supported subset documented on the package. That is deliberate: a silent
// skip would let a future schema revision introduce a construct this gate
// cannot see, and the gate would keep reporting success while checking less
// than it claims.
func ParseSchema(data []byte) (*Schema, error) {
	// Tolerate the UTF-8 BOM the original schema file carries.
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	var doc xsdSchemaDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse xsd: %w", err)
	}
	if doc.TargetNamespace == "" {
		return nil, fmt.Errorf("parse xsd: schema declares no targetNamespace")
	}
	if len(doc.Imports) > 0 || len(doc.Includes) > 0 {
		return nil, fmt.Errorf("parse xsd: xs:import/xs:include are outside the supported subset")
	}
	if len(doc.Groups) > 0 || len(doc.AttributeGroups) > 0 {
		return nil, fmt.Errorf("parse xsd: xs:group/xs:attributeGroup are outside the supported subset")
	}

	s := &Schema{
		TargetNamespace: doc.TargetNamespace,
		Version:         doc.Version,
		complexTypes:    make(map[string]*ComplexType, len(doc.ComplexTypes)),
		simpleTypes:     make(map[string]*SimpleType, len(doc.SimpleTypes)),
		elements:        make(map[string]string, len(doc.Elements)),
		effElements:     make(map[string][]Element),
		effAttrs:        make(map[string][]Attribute),
	}

	for i := range doc.SimpleTypes {
		st, err := convertSimpleType(&doc.SimpleTypes[i])
		if err != nil {
			return nil, err
		}
		if _, dup := s.simpleTypes[st.Name]; dup {
			return nil, fmt.Errorf("parse xsd: duplicate simpleType %q", st.Name)
		}
		s.simpleTypes[st.Name] = st
	}

	for i := range doc.ComplexTypes {
		ct, err := convertComplexType(&doc.ComplexTypes[i])
		if err != nil {
			return nil, err
		}
		if _, dup := s.complexTypes[ct.Name]; dup {
			return nil, fmt.Errorf("parse xsd: duplicate complexType %q", ct.Name)
		}
		s.complexTypes[ct.Name] = ct
	}

	for _, e := range doc.Elements {
		if e.Name == "" || e.Type == "" {
			return nil, fmt.Errorf("parse xsd: top-level element must have name and type (got name=%q type=%q)", e.Name, e.Type)
		}
		s.elements[e.Name] = e.Type
	}

	// Resolving every type up front turns a broken base reference or an
	// inheritance cycle into a parse-time error rather than a confusing
	// mid-validation failure.
	for name := range s.complexTypes {
		if _, err := s.EffectiveElements(name); err != nil {
			return nil, err
		}
		if _, err := s.EffectiveAttributes(name); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func convertSimpleType(in *xsdSimpleType) (*SimpleType, error) {
	if in.Union != nil {
		return nil, fmt.Errorf("parse xsd: simpleType %q uses xs:union, outside the supported subset", in.Name)
	}
	if in.List != nil {
		return nil, fmt.Errorf("parse xsd: simpleType %q uses xs:list, outside the supported subset", in.Name)
	}
	st := &SimpleType{Name: in.Name, MaxLength: -1}
	if in.Restriction == nil {
		return nil, fmt.Errorf("parse xsd: simpleType %q has no xs:restriction", in.Name)
	}
	st.Base = stripNS(in.Restriction.Base)
	if in.Restriction.MaxLength != nil {
		n, err := strconv.Atoi(in.Restriction.MaxLength.Value)
		if err != nil {
			return nil, fmt.Errorf("parse xsd: simpleType %q maxLength %q: %w", in.Name, in.Restriction.MaxLength.Value, err)
		}
		st.MaxLength = n
	}
	return st, nil
}

func convertComplexType(in *xsdComplexType) (*ComplexType, error) {
	if in.Name == "" {
		return nil, fmt.Errorf("parse xsd: anonymous top-level complexType is outside the supported subset")
	}
	if in.Abstract == "true" {
		return nil, fmt.Errorf("parse xsd: complexType %q is abstract, outside the supported subset", in.Name)
	}
	ct := &ComplexType{Name: in.Name}

	if err := rejectUnsupportedParticles(in.Name, in.Choice, in.All, in.GroupRef, in.AttrGroupRefs); err != nil {
		return nil, err
	}

	var (
		seq   *xsdSequence
		attrs []xsdAttribute
	)

	switch {
	case in.ComplexContent != nil:
		if in.ComplexContent.Restriction != nil {
			return nil, fmt.Errorf("parse xsd: complexType %q uses complexContent restriction, outside the supported subset", in.Name)
		}
		ext := in.ComplexContent.Extension
		if ext == nil {
			return nil, fmt.Errorf("parse xsd: complexType %q has complexContent with no extension", in.Name)
		}
		if err := rejectUnsupportedParticles(in.Name, ext.Choice, ext.All, ext.GroupRef, ext.AttrGroupRefs); err != nil {
			return nil, err
		}
		ct.Base = stripNS(ext.Base)
		seq, attrs = ext.Sequence, ext.Attributes

	case in.SimpleContent != nil:
		if in.SimpleContent.Restriction != nil {
			return nil, fmt.Errorf("parse xsd: complexType %q uses simpleContent restriction, outside the supported subset", in.Name)
		}
		ext := in.SimpleContent.Extension
		if ext == nil {
			return nil, fmt.Errorf("parse xsd: complexType %q has simpleContent with no extension", in.Name)
		}
		if err := rejectUnsupportedParticles(in.Name, ext.Choice, ext.All, ext.GroupRef, ext.AttrGroupRefs); err != nil {
			return nil, err
		}
		ct.SimpleContent = true
		ct.Base = stripNS(ext.Base)
		attrs = ext.Attributes

	default:
		seq, attrs = in.Sequence, in.Attributes
	}

	els, err := flattenSequence(in.Name, seq)
	if err != nil {
		return nil, err
	}
	ct.ownElements = els

	for _, a := range attrs {
		if a.Ref != "" {
			return nil, fmt.Errorf("parse xsd: complexType %q uses an attribute ref, outside the supported subset", in.Name)
		}
		ct.ownAttributes = append(ct.ownAttributes, Attribute{
			Name:     a.Name,
			Type:     stripNS(a.Type),
			Required: a.Use == "required",
			Owner:    in.Name,
		})
	}
	return ct, nil
}

func rejectUnsupportedParticles(owner string, choice, all, group *struct{}, attrGroups []struct{}) error {
	switch {
	case choice != nil:
		return fmt.Errorf("parse xsd: complexType %q uses xs:choice, outside the supported subset", owner)
	case all != nil:
		return fmt.Errorf("parse xsd: complexType %q uses xs:all, outside the supported subset", owner)
	case group != nil:
		return fmt.Errorf("parse xsd: complexType %q uses xs:group, outside the supported subset", owner)
	case len(attrGroups) > 0:
		return fmt.Errorf("parse xsd: complexType %q uses xs:attributeGroup, outside the supported subset", owner)
	}
	return nil
}

// flattenSequence converts a (possibly nested) xsd:sequence into a flat,
// ordered particle list. Nesting sequences does not change the required
// document order, so flattening is faithful for the maxOccurs="1" nesting
// sep.xsd uses.
func flattenSequence(owner string, seq *xsdSequence) ([]Element, error) {
	if seq == nil {
		return nil, nil
	}
	if seq.Choice != nil {
		return nil, fmt.Errorf("parse xsd: complexType %q has xs:choice in a sequence, outside the supported subset", owner)
	}
	if seq.All != nil {
		return nil, fmt.Errorf("parse xsd: complexType %q has xs:all in a sequence, outside the supported subset", owner)
	}
	if seq.GroupRef != nil {
		return nil, fmt.Errorf("parse xsd: complexType %q has xs:group in a sequence, outside the supported subset", owner)
	}
	if seq.Any != nil {
		return nil, fmt.Errorf("parse xsd: complexType %q has an xs:any wildcard, outside the supported subset", owner)
	}

	out := make([]Element, 0, len(seq.Elements))
	for _, e := range seq.Elements {
		if e.Ref != "" {
			return nil, fmt.Errorf("parse xsd: complexType %q uses an element ref, outside the supported subset", owner)
		}
		if e.ComplexType != nil || e.SimpleType != nil {
			return nil, fmt.Errorf("parse xsd: complexType %q declares an anonymous inline type for %q, outside the supported subset", owner, e.Name)
		}
		if e.Name == "" || e.Type == "" {
			return nil, fmt.Errorf("parse xsd: complexType %q has an element with no name or type", owner)
		}
		// XSD default for both minOccurs and maxOccurs is "1".
		required := e.MinOccurs == "" || e.MinOccurs != "0"
		repeatable := e.MaxOccurs != "" && e.MaxOccurs != "1"
		out = append(out, Element{
			Name:       e.Name,
			Type:       stripNS(e.Type),
			Required:   required,
			Repeatable: repeatable,
			Owner:      owner,
		})
	}
	for i := range seq.Nested {
		nested, err := flattenSequence(owner, &seq.Nested[i])
		if err != nil {
			return nil, err
		}
		out = append(out, nested...)
	}
	return out, nil
}

// EffectiveElements returns the full ordered sequence for typeName, with
// particles inherited through xsd:extension placed BEFORE the derived type's
// own particles. That ordering is the XSD extension rule and is precisely
// what an order-sensitive peer enforces.
func (s *Schema) EffectiveElements(typeName string) ([]Element, error) {
	if cached, ok := s.effElements[typeName]; ok {
		return cached, nil
	}
	chain, err := s.baseChain(typeName)
	if err != nil {
		return nil, err
	}
	var out []Element
	// chain is derived-first; walk it in reverse so the root base leads.
	for i := len(chain) - 1; i >= 0; i-- {
		out = append(out, chain[i].ownElements...)
	}
	s.effElements[typeName] = out
	return out, nil
}

// EffectiveAttributes returns all attributes for typeName including those
// inherited through xsd:extension.
func (s *Schema) EffectiveAttributes(typeName string) ([]Attribute, error) {
	if cached, ok := s.effAttrs[typeName]; ok {
		return cached, nil
	}
	chain, err := s.baseChain(typeName)
	if err != nil {
		return nil, err
	}
	var out []Attribute
	for i := len(chain) - 1; i >= 0; i-- {
		out = append(out, chain[i].ownAttributes...)
	}
	s.effAttrs[typeName] = out
	return out, nil
}

// baseChain returns typeName's inheritance chain, derived type first. It
// stops at a base that is not a schema complex type (a simple content base
// such as UInt8) and reports cycles rather than looping.
func (s *Schema) baseChain(typeName string) ([]*ComplexType, error) {
	var chain []*ComplexType
	seen := map[string]bool{}
	for name := typeName; name != ""; {
		ct, ok := s.complexTypes[name]
		if !ok {
			if name == typeName {
				return nil, fmt.Errorf("unknown complexType %q", typeName)
			}
			// Base is a simple type (simpleContent extension). Nothing
			// further to inherit structurally.
			break
		}
		if seen[name] {
			return nil, fmt.Errorf("inheritance cycle at complexType %q", name)
		}
		seen[name] = true
		chain = append(chain, ct)
		name = ct.Base
	}
	return chain, nil
}

// ComplexType returns the resolved complex type, or false if absent.
func (s *Schema) ComplexType(name string) (*ComplexType, bool) {
	ct, ok := s.complexTypes[name]
	return ct, ok
}

// SimpleType returns the resolved simple type, or false if absent.
func (s *Schema) SimpleType(name string) (*SimpleType, bool) {
	st, ok := s.simpleTypes[name]
	return st, ok
}

// TypeOfElement returns the type name declared for a top-level element.
func (s *Schema) TypeOfElement(name string) (string, bool) {
	t, ok := s.elements[name]
	return t, ok
}

// TopLevelElements returns every top-level element name, sorted, so callers
// can enumerate the schema deterministically.
func (s *Schema) TopLevelElements() []string {
	out := make([]string, 0, len(s.elements))
	for name := range s.elements {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// resolveBuiltin walks a simple type's restriction chain down to the XSD
// built-in it bottoms out at, accumulating the tightest maxLength facet seen.
// It returns the built-in name (for example "hexBinary" or "unsignedByte")
// and the effective maxLength, or -1 when unconstrained.
func (s *Schema) resolveBuiltin(typeName string) (builtin string, maxLen int) {
	maxLen = -1
	seen := map[string]bool{}
	name := typeName
	for {
		if seen[name] {
			return "", maxLen
		}
		seen[name] = true

		st, ok := s.simpleTypes[name]
		if !ok {
			// Not a schema simple type: either an XSD built-in or a
			// complex type we do not treat as a value.
			if isXSDBuiltin(name) {
				return name, maxLen
			}
			return "", maxLen
		}
		if st.MaxLength >= 0 && (maxLen < 0 || st.MaxLength < maxLen) {
			maxLen = st.MaxLength
		}
		name = st.Base
	}
}

func stripNS(qname string) string {
	if i := strings.IndexByte(qname, ':'); i >= 0 {
		return qname[i+1:]
	}
	return qname
}
