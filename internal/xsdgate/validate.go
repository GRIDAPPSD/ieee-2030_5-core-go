package xsdgate

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// ProblemKind classifies a schema violation. Tests can assert on the kind
// rather than on message text, so error wording can be improved without
// churning assertions.
type ProblemKind string

const (
	// KindOrder means elements appeared out of xsd:sequence order.
	KindOrder ProblemKind = "order"
	// KindMissingElement means a minOccurs>=1 element was absent.
	KindMissingElement ProblemKind = "missing-element"
	// KindMissingAttribute means a use="required" attribute was absent.
	KindMissingAttribute ProblemKind = "missing-attribute"
	// KindUnknownElement means an element is not declared on the type.
	KindUnknownElement ProblemKind = "unknown-element"
	// KindUnknownAttribute means an attribute is not declared on the type.
	KindUnknownAttribute ProblemKind = "unknown-attribute"
	// KindPlacement means a name is declared by the schema, but on the other
	// axis: an attribute emitted as an element, or an element emitted as an
	// attribute.
	KindPlacement ProblemKind = "placement"
	// KindRepetition means a maxOccurs="1" element appeared more than once.
	KindRepetition ProblemKind = "repetition"
	// KindLexical means a value is outside its simple type's lexical space.
	KindLexical ProblemKind = "lexical"
	// KindNamespace means the root element carried the wrong namespace.
	KindNamespace ProblemKind = "namespace"
)

// Problem is a single schema violation.
type Problem struct {
	Kind ProblemKind
	// Path is a slash-separated element path, for example
	// "DERCapability/rtgMaxW/value".
	Path string
	// Message explains the violation in terms a reader can act on.
	Message string
}

func (p Problem) String() string {
	return fmt.Sprintf("[%s] %s: %s", p.Kind, p.Path, p.Message)
}

// Problems is an ordered collection of violations.
type Problems []Problem

// Error renders all problems as a single multi-line string. It returns "" for
// an empty collection.
func (ps Problems) Error() string {
	if len(ps) == 0 {
		return ""
	}
	var b strings.Builder
	for i, p := range ps {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("  ")
		b.WriteString(p.String())
	}
	return b.String()
}

// Has reports whether any problem has the given kind.
func (ps Problems) Has(kind ProblemKind) bool {
	for _, p := range ps {
		if p.Kind == kind {
			return true
		}
	}
	return false
}

// node is a decoded XML element.
type node struct {
	name     xml.Name
	attrs    []xml.Attr
	children []*node
	text     string
}

// Validate checks a marshalled document against the schema type typeName.
//
// typeName is the SCHEMA type name (for example "DERCapability"), which for
// this schema is also the top-level element name. Validate returns nil when
// the document is acceptable under the checks this package implements. See
// the package doc for the explicit list of what is and is not checked: a nil
// return is not proof of full XSD validity.
func (s *Schema) Validate(typeName string, data []byte) (Problems, error) {
	root, err := parseDocument(data)
	if err != nil {
		return nil, err
	}
	if _, ok := s.complexTypes[typeName]; !ok {
		return nil, fmt.Errorf("xsdgate: %q is not a complexType in the schema", typeName)
	}

	var ps Problems
	if root.name.Space != s.TargetNamespace {
		ps = append(ps, Problem{
			Kind: KindNamespace,
			Path: root.name.Local,
			Message: fmt.Sprintf("root element namespace is %q, want the schema targetNamespace %q",
				root.name.Space, s.TargetNamespace),
		})
	}

	ps = append(ps, s.validateNode(root, typeName, root.name.Local, true)...)
	if len(ps) == 0 {
		return nil, nil
	}
	return ps, nil
}

// ValidateElement is Validate keyed by top-level element name, resolving the
// element's declared type from the schema.
func (s *Schema) ValidateElement(elementName string, data []byte) (Problems, error) {
	typeName, ok := s.elements[elementName]
	if !ok {
		return nil, fmt.Errorf("xsdgate: %q is not a top-level element in the schema", elementName)
	}
	return s.Validate(typeName, data)
}

// validateNode checks n against typeName. isRoot marks the document's root
// element, which gets the narrow schemaVer tolerance (see checkAttributes);
// every descendant is validated with isRoot false.
func (s *Schema) validateNode(n *node, typeName, path string, isRoot bool) Problems {
	var ps Problems

	ct, ok := s.complexTypes[typeName]
	if !ok {
		// Leaf value: the element's type is a simple type.
		if msg := s.checkLexical(typeName, n.text); msg != "" {
			ps = append(ps, Problem{Kind: KindLexical, Path: path, Message: msg})
		}
		// A simple-typed element must not carry children.
		for _, c := range n.children {
			ps = append(ps, Problem{
				Kind:    KindUnknownElement,
				Path:    path + "/" + c.name.Local,
				Message: fmt.Sprintf("type %s is a simple type and permits no child elements, but %q was emitted", typeName, c.name.Local),
			})
		}
		return ps
	}

	elems, err := s.EffectiveElements(typeName)
	if err != nil {
		return append(ps, Problem{Kind: KindUnknownElement, Path: path, Message: err.Error()})
	}
	attrs, err := s.EffectiveAttributes(typeName)
	if err != nil {
		return append(ps, Problem{Kind: KindUnknownAttribute, Path: path, Message: err.Error()})
	}

	elemByName := make(map[string]Element, len(elems))
	elemIndex := make(map[string]int, len(elems))
	for i, e := range elems {
		elemByName[e.Name] = e
		elemIndex[e.Name] = i
	}
	attrByName := make(map[string]Attribute, len(attrs))
	for _, a := range attrs {
		attrByName[a.Name] = a
	}

	ps = append(ps, s.checkAttributes(n, path, typeName, attrByName, elemByName, isRoot)...)

	// simpleContent types carry a value rather than children.
	if ct.SimpleContent {
		if msg := s.checkLexical(ct.Base, n.text); msg != "" {
			ps = append(ps, Problem{Kind: KindLexical, Path: path, Message: msg})
		}
	}

	ps = append(ps, s.checkChildren(n, path, typeName, elems, elemByName, elemIndex, attrByName)...)
	return ps
}

// rootTolerated is the set of attribute names accepted on the document's
// root element even though the 2.1 schema this gate checks against does not
// declare them. It exists for exactly one entry: IEEE 2030.5-2023 clause
// 5.6.2 REQUIRES every payload's top-level element to carry schemaVer, a
// requirement the 2.1 schema predates. Rejecting a 2023 peer for supplying
// an attribute its edition of the standard obliges it to send would be
// wrong, so the gate treats schemaVer on the root as tolerated rather than
// unknown. Nothing else is in this set: an unrelated unknown attribute on
// the root is still a defect, and schemaVer anywhere other than the root is
// still unknown too (see checkAttributes' isRoot parameter). See
// IEEECORE-078.
var rootTolerated = map[string]bool{
	"schemaVer": true,
}

func (s *Schema) checkAttributes(n *node, path, typeName string, attrByName map[string]Attribute, elemByName map[string]Element, isRoot bool) Problems {
	var ps Problems
	seen := map[string]bool{}

	for _, a := range n.attrs {
		if isNamespaceDecl(a.Name) || a.Name.Space == "http://www.w3.org/2001/XMLSchema-instance" {
			continue
		}
		local := a.Name.Local
		seen[local] = true

		if decl, ok := attrByName[local]; ok {
			if msg := s.checkLexical(decl.Type, a.Value); msg != "" {
				ps = append(ps, Problem{Kind: KindLexical, Path: path + "/@" + local, Message: msg})
			}
			continue
		}
		if el, ok := elemByName[local]; ok {
			ps = append(ps, Problem{
				Kind: KindPlacement,
				Path: path + "/@" + local,
				Message: fmt.Sprintf("schema declares %q on %s as a CHILD ELEMENT (type %s, declared by %s), but it was emitted as an attribute",
					local, typeName, el.Type, el.Owner),
			})
			continue
		}
		if isRoot && rootTolerated[local] {
			continue
		}
		ps = append(ps, Problem{
			Kind:    KindUnknownAttribute,
			Path:    path + "/@" + local,
			Message: fmt.Sprintf("attribute %q is not declared on type %s", local, typeName),
		})
	}

	for _, a := range sortedAttrs(attrByName) {
		if a.Required && !seen[a.Name] {
			ps = append(ps, Problem{
				Kind: KindMissingAttribute,
				Path: path + "/@" + a.Name,
				Message: fmt.Sprintf("attribute %q is use=\"required\" on %s (declared by %s) but was not emitted",
					a.Name, typeName, a.Owner),
			})
		}
	}
	return ps
}

func (s *Schema) checkChildren(
	n *node,
	path, typeName string,
	elems []Element,
	elemByName map[string]Element,
	elemIndex map[string]int,
	attrByName map[string]Attribute,
) Problems {
	var ps Problems

	counts := map[string]int{}
	lastIdx := -1
	lastName := ""

	for _, c := range n.children {
		local := c.name.Local
		childPath := path + "/" + local

		decl, ok := elemByName[local]
		if !ok {
			if a, isAttr := attrByName[local]; isAttr {
				ps = append(ps, Problem{
					Kind: KindPlacement,
					Path: childPath,
					Message: fmt.Sprintf("schema declares %q on %s as an ATTRIBUTE (type %s, declared by %s), but it was emitted as a child element",
						local, typeName, a.Type, a.Owner),
				})
				continue
			}
			ps = append(ps, Problem{
				Kind:    KindUnknownElement,
				Path:    childPath,
				Message: fmt.Sprintf("element %q is not declared on type %s; the schema has no such element on this type", local, typeName),
			})
			continue
		}

		counts[local]++
		idx := elemIndex[local]
		if idx < lastIdx {
			ps = append(ps, Problem{
				Kind: KindOrder,
				Path: childPath,
				Message: fmt.Sprintf("element %q (sequence position %d) appears after %q (sequence position %d); xsd:sequence order on %s is: %s",
					local, idx, lastName, lastIdx, typeName, sequenceSummary(elems)),
			})
		}
		if idx > lastIdx {
			lastIdx = idx
			lastName = local
		}

		ps = append(ps, s.validateNode(c, decl.Type, childPath, false)...)
	}

	for name, count := range counts {
		if count > 1 && !elemByName[name].Repeatable {
			ps = append(ps, Problem{
				Kind:    KindRepetition,
				Path:    path + "/" + name,
				Message: fmt.Sprintf("element %q has maxOccurs=\"1\" on %s but appeared %d times", name, typeName, count),
			})
		}
	}

	for _, e := range elems {
		if e.Required && counts[e.Name] == 0 {
			ps = append(ps, Problem{
				Kind: KindMissingElement,
				Path: path + "/" + e.Name,
				Message: fmt.Sprintf("element %q is minOccurs=\"1\" on %s (type %s, declared by %s) but was not emitted; "+
					"a required element missing from output is usually an omitempty tag on a required field",
					e.Name, typeName, e.Type, e.Owner),
			})
		}
	}
	return ps
}

func sequenceSummary(elems []Element) string {
	names := make([]string, 0, len(elems))
	for _, e := range elems {
		if e.Required {
			names = append(names, e.Name+"!")
			continue
		}
		names = append(names, e.Name)
	}
	return strings.Join(names, ", ")
}

func sortedAttrs(m map[string]Attribute) []Attribute {
	out := make([]Attribute, 0, len(m))
	for _, a := range m {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func isNamespaceDecl(n xml.Name) bool {
	return n.Local == "xmlns" || n.Space == "xmlns"
}

// parseDocument decodes a document into a node tree, preserving child order.
func parseDocument(data []byte) (*node, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var stack []*node
	var root *node

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("xsdgate: decode document: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &node{name: t.Name, attrs: append([]xml.Attr(nil), t.Attr...)}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, n)
			} else if root != nil {
				return nil, fmt.Errorf("xsdgate: document has more than one root element")
			} else {
				root = n
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("xsdgate: unbalanced end element %q", t.Name.Local)
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text += string(t)
			}
		}
	}
	if root == nil {
		return nil, fmt.Errorf("xsdgate: document has no root element")
	}
	return root, nil
}
