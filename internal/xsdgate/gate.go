package xsdgate

import (
	"encoding/xml"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/schema"
)

var (
	loadOnce sync.Once
	loaded   *Schema
	loadErr  error
)

// Load reads and parses the normative IEEE 2030.5 schema, once per process.
//
// The schema is supplied by the operator rather than distributed with this
// project; see package schema and the NOTICE file. When no copy is
// available the error wraps schema.ErrNotFound.
//
// Parsing the whole 6922-line schema takes a few milliseconds, so memoising
// keeps the gate cheap enough to apply to every marshalling test rather than
// only to a curated few.
func Load() (*Schema, error) {
	loadOnce.Do(func() {
		data, _, err := schema.Load()
		if err != nil {
			loadErr = err
			return
		}
		loaded, loadErr = ParseSchema(data)
	})
	return loaded, loadErr
}

// MustLoad is Load for tests. It SKIPS the calling test when no schema copy
// is available, so a contributor without an IEEE copy runs the suite green,
// and it FAILS for every other error, so a wrong, unreadable, or unparseable
// schema is never mistaken for a pass.
//
// Setting schema.EnvRequired turns the skip into a failure. That is the
// switch CI flips once it supplies a schema: without it, a rotated secret or
// a typo in the path variable would skip the whole gate and still report
// green, which is the exact failure mode the gate exists to prevent.
func MustLoad(t *testing.T) *Schema {
	t.Helper()

	// Checked before the load result so a malformed SEP2_SCHEMA_REQUIRED is
	// reported even on a run where a schema happens to be present.
	required, rerr := schema.Required()
	if rerr != nil {
		t.Fatalf("schema gate configuration: %v", rerr)
	}

	s, err := Load()
	switch {
	case err == nil:
		return s
	case errors.Is(err, schema.ErrNotFound) && !required:
		t.Skipf("schema-gated test skipped: %v (set %s=1 to make this a failure)", err, schema.EnvRequired)
	case errors.Is(err, schema.ErrNotFound):
		t.Fatalf("%s is set but no schema is available: %v", schema.EnvRequired, err)
	default:
		t.Fatalf("load IEEE 2030.5 schema: %v", err)
	}
	return nil
}

// AssertValid marshals v, validates the result against the schema type
// typeName, and fails the test with the offending XML and every violation if
// it does not conform.
//
// This is the entry point resource tests should use. It reports ALL problems
// rather than stopping at the first, so a single run shows the whole gap
// between a type and the standard.
func AssertValid(t *testing.T, typeName string, v any) {
	t.Helper()
	s := MustLoad(t)

	data, err := xml.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("%s: marshal failed: %v", typeName, err)
	}
	problems, err := s.Validate(typeName, data)
	if err != nil {
		t.Fatalf("%s: validate failed: %v", typeName, err)
	}
	if len(problems) > 0 {
		t.Errorf("%s does not conform to the normative IEEE 2030.5 schema (%d problem(s)):\n%s\n\nmarshalled XML:\n%s",
			typeName, len(problems), problems.Error(), data)
	}
}

// CollectProblems marshals v and returns the schema violations without
// failing the test. Use it to pin a KNOWN-FAILING resource: the test asserts
// that the specific known defects are still present, so the gate stays green
// while the defect stays visible, and the test breaks loudly the moment
// someone fixes the resource and forgets to unpin it.
func CollectProblems(t *testing.T, typeName string, v any) (Problems, []byte) {
	t.Helper()
	s := MustLoad(t)

	data, err := xml.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("%s: marshal failed: %v", typeName, err)
	}
	problems, err := s.Validate(typeName, data)
	if err != nil {
		t.Fatalf("%s: validate failed: %v", typeName, err)
	}
	return problems, data
}

// CollectStructProblems runs the static struct-tag check and returns the
// violations without failing the test.
func CollectStructProblems(t *testing.T, typeName string, v any) Problems {
	t.Helper()
	s := MustLoad(t)

	problems, err := s.CheckStruct(typeName, reflect.TypeOf(v))
	if err != nil {
		t.Fatalf("%s: struct check failed: %v", typeName, err)
	}
	return problems
}

// AssertStructValid fails the test if the Go struct definition disagrees with
// the schema type.
func AssertStructValid(t *testing.T, typeName string, v any) {
	t.Helper()
	if problems := CollectStructProblems(t, typeName, v); len(problems) > 0 {
		t.Errorf("%s struct definition disagrees with the normative IEEE 2030.5 schema (%d problem(s)):\n%s",
			typeName, len(problems), problems.Error())
	}
}

// Summary renders problems as "kind path" lines, sorted, for stable
// comparison against a pinned expectation.
func (ps Problems) Summary() []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, fmt.Sprintf("%s %s", p.Kind, p.Path))
	}
	return out
}
