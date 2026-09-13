package xsdgate

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// nestedFixtureTypes are added to integerFixtureXSD so the descent into child
// element types is exercised without sep.xsd.
const nestedFixtureTypes = `
  <xs:complexType name="Leaf">
    <xs:sequence><xs:element name="n" type="Int16" minOccurs="0"/></xs:sequence>
    <xs:attribute name="a" type="UInt8"/>
  </xs:complexType>
  <xs:complexType name="MidBase">
    <xs:sequence><xs:element name="m" type="Int16" minOccurs="0"/></xs:sequence>
  </xs:complexType>
  <xs:complexType name="Mid">
    <xs:complexContent><xs:extension base="MidBase"><xs:sequence>
      <xs:element name="leaf" type="Leaf" minOccurs="0" maxOccurs="unbounded"/>
      <xs:element name="curve" type="CurveLink" minOccurs="0"/>
      <xs:element name="scalar" type="Int16" minOccurs="0"/>
    </xs:sequence></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:complexType name="CurveLink"><xs:attribute name="href" type="xs:anyURI"/></xs:complexType>
  <xs:complexType name="Node">
    <xs:sequence>
      <xs:element name="v" type="Int16" minOccurs="0"/>
      <xs:element name="next" type="Node" minOccurs="0"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="Root">
    <xs:sequence>
      <xs:element name="mid" type="Mid" minOccurs="0"/>
      <xs:element name="node" type="Node" minOccurs="0"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="Siblings">
    <xs:sequence>
      <xs:element name="a" type="Leaf" minOccurs="0"/>
      <xs:element name="b" type="Leaf" minOccurs="0"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="SelfRoot">
    <xs:sequence>
      <xs:element name="child" type="SelfRoot" minOccurs="0"/>
      <xs:element name="n" type="Int16" minOccurs="0"/>
    </xs:sequence>
  </xs:complexType>
`

type nestedLeaf struct {
	N int32 `xml:"n,omitempty"`
	A int16 `xml:"a,attr,omitempty"`
}

type nestedLeafOK struct {
	N int16 `xml:"n,omitempty"`
	A uint8 `xml:"a,attr,omitempty"`
}

// nestedLeafUndeclaredAttr binds an attribute name Leaf does not declare, so
// the child's own undeclared-attribute path is exercised below the top level.
type nestedLeafUndeclaredAttr struct {
	A uint8 `xml:"a,attr,omitempty"`
	Z int32 `xml:"z,attr,omitempty"`
}

// nestedLeafUndeclaredNonInt binds an undeclared child element that holds no
// Go integer, which must stay silent rather than reported as unresolved.
type nestedLeafUndeclaredNonInt struct {
	A     uint8  `xml:"a,attr,omitempty"`
	Extra string `xml:"extra,omitempty"`
}

// nestedScalarSkippedInt's only integer-holding field is tagged xml:"-", so
// it must not count toward holdsInteger.
type nestedScalarSkippedInt struct {
	X       string `xml:"x,omitempty"`
	Skipped int32  `xml:"-"`
}

// siblingLeaf is bound to Leaf under two sibling fields of the same parent,
// to prove the active-set cleanup runs after each child, not once overall.
type siblingLeaf struct {
	N int32 `xml:"n,omitempty"`
}

type NestedMidBase struct {
	M int32 `xml:"m,omitempty"`
}

type NestedMidBaseOK struct {
	M int16 `xml:"m,omitempty"`
}

type nestedTextOnly struct {
	S string `xml:"s,omitempty"`
}

type nestedTextNode struct {
	S    string          `xml:"s,omitempty"`
	Next *nestedTextNode `xml:"next,omitempty"`
}

type nestedNode struct {
	V    int32       `xml:"v,omitempty"`
	Next *nestedNode `xml:"next,omitempty"`
}

type (
	midDepth2 struct {
		M int32 `xml:"m,omitempty"`
	}
	rootDepth2 struct {
		Mid *midDepth2 `xml:"mid,omitempty"`
	}

	midDepth3 struct {
		Leaf []*nestedLeaf `xml:"leaf,omitempty"`
	}
	rootDepth3 struct {
		Mid midDepth3 `xml:"mid"`
	}

	midArray struct {
		Leaf [2]nestedLeaf `xml:"leaf"`
	}
	rootArray struct {
		Mid *midArray `xml:"mid,omitempty"`
	}

	midEmbedded struct {
		NestedMidBase
		Leaf []nestedLeafOK `xml:"leaf,omitempty"`
	}
	rootEmbedded struct {
		Mid *midEmbedded `xml:"mid,omitempty"`
	}

	midClean struct {
		NestedMidBaseOK
		Leaf   []*nestedLeafOK `xml:"leaf,omitempty"`
		Scalar *nestedTextOnly `xml:"scalar,omitempty"`
	}
	rootClean struct {
		Mid  []midClean `xml:"mid,omitempty"`
		Node *struct {
			V int16 `xml:"v,omitempty"`
		} `xml:"node,omitempty"`
	}

	midUndeclared struct {
		X     int32         `xml:"x,omitempty"`
		Extra *nestedLeafOK `xml:"extra,omitempty"`
		Deep  *struct {
			Leaf nestedLeafOK `xml:"leaf"`
		} `xml:"deep,omitempty"`
	}
	rootUndeclared struct {
		Mid *midUndeclared `xml:"mid,omitempty"`
	}

	midScalarStruct struct {
		Scalar *nestedLeafOK `xml:"scalar,omitempty"`
	}
	rootScalarStruct struct {
		Mid *midScalarStruct `xml:"mid,omitempty"`
	}

	midScalarRecursive struct {
		Scalar *nestedTextNode `xml:"scalar,omitempty"`
	}
	rootScalarRecursive struct {
		Mid *midScalarRecursive `xml:"mid,omitempty"`
	}

	midCurve struct {
		Curve *int32 `xml:"curve,omitempty"`
	}
	rootCurve struct {
		Mid *midCurve `xml:"mid,omitempty"`
	}

	rootCycle struct {
		Node *nestedNode `xml:"node,omitempty"`
	}

	midLeafUndeclaredAttr struct {
		Leaf *nestedLeafUndeclaredAttr `xml:"leaf,omitempty"`
	}
	rootLeafUndeclaredAttr struct {
		Mid *midLeafUndeclaredAttr `xml:"mid,omitempty"`
	}

	midLeafUndeclaredNonInt struct {
		Leaf *nestedLeafUndeclaredNonInt `xml:"leaf,omitempty"`
	}
	rootLeafUndeclaredNonInt struct {
		Mid *midLeafUndeclaredNonInt `xml:"mid,omitempty"`
	}

	midScalarSkippedInt struct {
		Scalar *nestedScalarSkippedInt `xml:"scalar,omitempty"`
	}
	rootScalarSkippedInt struct {
		Mid *midScalarSkippedInt `xml:"mid,omitempty"`
	}

	siblings struct {
		A *siblingLeaf `xml:"a,omitempty"`
		B *siblingLeaf `xml:"b,omitempty"`
	}

	selfRoot struct {
		Child *selfRoot `xml:"child,omitempty"`
		N     int32     `xml:"n,omitempty"`
	}
)

func nestedSchema(t *testing.T) *Schema {
	t.Helper()
	doc := strings.Replace(integerFixtureXSD, "</xs:schema>", nestedFixtureTypes+"</xs:schema>", 1)
	s, err := ParseSchema([]byte(doc))
	if err != nil {
		t.Fatalf("parse fixture schema: %v", err)
	}
	return s
}

func sortedIntegerProblems(ps Problems) []string {
	got := integerProblems(ps)
	sort.Strings(got)
	return got
}

func TestCheckStructIntegerDescendsIntoChildElements(t *testing.T) {
	t.Parallel()
	s := nestedSchema(t)
	cases := []struct {
		name string
		root any
		want []string
	}{
		{"depth 2 through a pointer child", rootDepth2{}, []string{"integer-width Root.Mid.M"}},
		{"depth 3 through a slice of pointers, element and attribute", rootDepth3{}, []string{
			"integer-width Root.Mid.Leaf.A",
			"integer-width Root.Mid.Leaf.N",
		}},
		{"depth 3 through an array of structs", rootArray{}, []string{
			"integer-width Root.Mid.Leaf.A",
			"integer-width Root.Mid.Leaf.N",
		}},
		{"embedded struct in a child is flattened", rootEmbedded{}, []string{"integer-width Root.Mid.M"}},
		{"conforming children at every depth", rootClean{}, nil},
		{"undeclared integer and integer-holding struct in a child", rootUndeclared{}, []string{
			"integer-unresolved Root.Mid.Deep",
			"integer-unresolved Root.Mid.Extra",
			"integer-unresolved Root.Mid.X",
		}},
		{"integer-holding struct over a simple type", rootScalarStruct{}, []string{"integer-unresolved Root.Mid.Scalar"}},
		{"recursive struct with no integers over a simple type", rootScalarRecursive{}, nil},
		{"integer over a child complex type with no simple content", rootCurve{}, []string{"integer-unresolved Root.Mid.Curve"}},
		{"recursive child type terminates", rootCycle{}, []string{"integer-width Root.Node.V"}},
		{"undeclared attribute on a child type", rootLeafUndeclaredAttr{}, []string{"integer-unresolved Root.Mid.Leaf.Z"}},
		{"undeclared non-integer field on a child type is not reported", rootLeafUndeclaredNonInt{}, nil},
		{"skip-tagged integer field does not count toward holds-integer", rootScalarSkippedInt{}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ps, err := s.CheckStruct("Root", reflect.TypeOf(tc.root))
			if err != nil {
				t.Fatalf("CheckStruct: %v", err)
			}
			if got := sortedIntegerProblems(ps); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("integer problems = %v, want %v (all problems: %v)", got, tc.want, ps.Summary())
			}
		})
	}
}

// TestCheckStructNestedMessageNamesElementPath pins that a nested report names
// the element path and the child type, since the Go path alone does not say
// which schema declaration was compared.
func TestCheckStructNestedMessageNamesElementPath(t *testing.T) {
	t.Parallel()
	s := nestedSchema(t)
	ps, err := s.CheckStruct("Root", reflect.TypeOf(rootDepth3{}))
	if err != nil {
		t.Fatalf("CheckStruct: %v", err)
	}
	want := map[string][]string{
		"Root.Mid.Leaf.N": {"Root/mid/leaf/n", "on Leaf", "Int16 -> short"},
		"Root.Mid.Leaf.A": {"Root/mid/leaf/@a", "on Leaf", "UInt8 -> unsignedByte"},
	}
	for _, p := range ps {
		subs, ok := want[p.Path]
		if !ok {
			continue
		}
		delete(want, p.Path)
		for _, sub := range subs {
			if !strings.Contains(p.Message, sub) {
				t.Errorf("%s message %q does not contain %q", p.Path, p.Message, sub)
			}
		}
	}
	for path := range want {
		t.Errorf("no problem reported at %s (all problems: %v)", path, ps.Summary())
	}
}

// TestCheckStructUnresolvedMessageNamesElementPath is
// TestCheckStructNestedMessageNamesElementPath's counterpart for the
// unresolved message, which names its own path fields independently.
func TestCheckStructUnresolvedMessageNamesElementPath(t *testing.T) {
	t.Parallel()
	s := nestedSchema(t)
	ps, err := s.CheckStruct("Root", reflect.TypeOf(rootCurve{}))
	if err != nil {
		t.Fatalf("CheckStruct: %v", err)
	}
	for _, p := range ps {
		if p.Path != "Root.Mid.Curve" {
			continue
		}
		if !strings.Contains(p.Message, "Root/mid/curve") {
			t.Errorf("message %q does not name the element path Root/mid/curve", p.Message)
		}
		return
	}
	t.Fatalf("no problem at Root.Mid.Curve (all problems: %v)", ps.Summary())
}

// TestCheckStructSiblingChildTypeEachChecked pins that active is cleared
// after each child descent, so two sibling fields sharing a (Go type, schema
// type) pair are each checked instead of only the first.
func TestCheckStructSiblingChildTypeEachChecked(t *testing.T) {
	t.Parallel()
	s := nestedSchema(t)
	ps, err := s.CheckStruct("Siblings", reflect.TypeOf(siblings{}))
	if err != nil {
		t.Fatalf("CheckStruct: %v", err)
	}
	want := []string{"integer-width Siblings.A.N", "integer-width Siblings.B.N"}
	if got := sortedIntegerProblems(ps); !reflect.DeepEqual(got, want) {
		t.Errorf("integer problems = %v, want %v (all problems: %v)", got, want, ps.Summary())
	}
}

// TestCheckStructRootPairSeededBeforeDescent pins that the top-level (Go
// type, schema type) pair is seeded into active before the descent starts, so
// a child element referring back to the exact same pair is not independently
// re-checked and reported a second time.
func TestCheckStructRootPairSeededBeforeDescent(t *testing.T) {
	t.Parallel()
	s := nestedSchema(t)
	ps, err := s.CheckStruct("SelfRoot", reflect.TypeOf(selfRoot{}))
	if err != nil {
		t.Fatalf("CheckStruct: %v", err)
	}
	want := []string{"integer-width SelfRoot.N"}
	if got := sortedIntegerProblems(ps); !reflect.DeepEqual(got, want) {
		t.Errorf("integer problems = %v, want %v (all problems: %v)", got, want, ps.Summary())
	}
}
