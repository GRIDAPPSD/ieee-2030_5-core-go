package schema_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/schema"
)

// These tests exercise the locator itself and deliberately never need a real
// copy of sep.xsd. They are the part of the schema story that must stay
// green for a contributor who has no IEEE copy at all.

// emptyModule returns a directory that looks like a module root but holds no
// schema, and makes it the working directory.
func emptyModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return dir
}

// TestResolveDistinguishesAbsentFromMisconfigured is the load-bearing
// behaviour of the locator. Absence is a skip, so it must be reported as
// ErrNotFound; a path that was explicitly configured and does not resolve is
// an operator error, so it must NOT be, or a broken CI secret would
// masquerade as "no schema here" and skip the gate while reporting green.
func TestResolveDistinguishesAbsentFromMisconfigured(t *testing.T) {
	dir := emptyModule(t)

	tests := []struct {
		name        string
		path        string
		wantMissing bool // errors.Is(err, ErrNotFound)
	}{
		{
			name:        "unset falls through to the default path and finds nothing",
			path:        "",
			wantMissing: true,
		},
		{
			name:        "configured path that does not exist",
			path:        filepath.Join(dir, "no-such-file.xsd"),
			wantMissing: false,
		},
		{
			name:        "configured path that is a directory",
			path:        dir,
			wantMissing: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(schema.EnvPath, tc.path)

			got, err := schema.Resolve()
			if err == nil {
				t.Fatalf("Resolve() = %q, want an error", got)
			}
			if missing := errors.Is(err, schema.ErrNotFound); missing != tc.wantMissing {
				t.Errorf("errors.Is(err, ErrNotFound) = %v, want %v; err = %v", missing, tc.wantMissing, err)
			}
		})
	}
}

// TestResolveDefaultPath asserts the conventional in-repo location resolves
// relative to the module root, not to the test's own package directory.
// Tests run with the working directory set to their package, so a plain
// relative path would only work for tests in the root.
func TestResolveDefaultPath(t *testing.T) {
	dir := emptyModule(t)
	t.Setenv(schema.EnvPath, "")

	want := filepath.Join(dir, schema.DefaultPath)
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, []byte("<xs:schema/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Nested deeper than the root, the way a package's tests run.
	sub := filepath.Join(dir, "internal", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)

	got, err := schema.Resolve()
	if err != nil {
		t.Fatalf("Resolve(): %v", err)
	}
	if got != want {
		t.Errorf("Resolve() = %q, want %q", got, want)
	}
}

// TestLoadRejectsWrongDocument proves the integrity check has teeth: a file
// that is not the pinned schema fails with a message naming what was found,
// rather than parsing and producing a wall of confusing validation errors.
func TestLoadRejectsWrongDocument(t *testing.T) {
	emptyModule(t)

	path := filepath.Join(t.TempDir(), "wrong.xsd")
	body := []byte("\xef\xbb\xbf<?xml version=\"1.0\"?>\r\n<xs:schema/>\r\n")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(schema.EnvPath, path)

	_, gotPath, err := schema.Load()
	if err == nil {
		t.Fatal("Load() succeeded on a document that is not the IEEE 2030.5 schema")
	}
	if errors.Is(err, schema.ErrNotFound) {
		t.Errorf("wrong document reported as absent: %v", err)
	}
	var ie *schema.IntegrityError
	if !errors.As(err, &ie) {
		t.Fatalf("Load() error = %T (%v), want *schema.IntegrityError", err, err)
	}
	if ie.Path != path {
		t.Errorf("IntegrityError.Path = %q, want %q", ie.Path, path)
	}
	if ie.SHA256 == schema.NormalizedSHA256 {
		t.Error("IntegrityError reports the expected digest, so nothing was actually wrong")
	}
	// Normalization strips the BOM (3 bytes) and both CRs.
	if got, want := ie.Size, len(body)-3-2; got != want {
		t.Errorf("IntegrityError.Size = %d, want %d (normalized length)", got, want)
	}
	if gotPath != path {
		t.Errorf("Load() path = %q, want %q", gotPath, path)
	}
}

// TestRequired covers the switch that keeps an absent schema from being
// silently satisfiable. A value that is not a boolean is an error rather
// than a false: SEP2_SCHEMA_REQUIRED=ture must not quietly disarm the gate.
func TestRequired(t *testing.T) {
	tests := []struct {
		raw     string
		want    bool
		wantErr bool
	}{
		{raw: "", want: false},
		{raw: "1", want: true},
		{raw: "true", want: true},
		{raw: "TRUE", want: true},
		{raw: " 1 ", want: true},
		{raw: "0", want: false},
		{raw: "false", want: false},
		{raw: "ture", wantErr: true},
		{raw: "yes", wantErr: true},
	}

	for _, tc := range tests {
		t.Run("value "+tc.raw, func(t *testing.T) {
			t.Setenv(schema.EnvRequired, tc.raw)

			got, err := schema.Required()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Required() = %v, want an error for %q", got, tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("Required(): %v", err)
			}
			if got != tc.want {
				t.Errorf("Required() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestNormalize pins the transformation the pinned digest is taken over.
func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "bom and crlf", in: "\xef\xbb\xbf<a>\r\n</a>\r\n", want: "<a>\n</a>\n"},
		{name: "already normalized", in: "<a>\n</a>\n", want: "<a>\n</a>\n"},
		{name: "lone cr", in: "<a>\r</a>", want: "<a></a>"},
		{name: "bom only at the start", in: "<a>\xef\xbb\xbf</a>", want: "<a>\xef\xbb\xbf</a>"},
		{name: "empty", in: "", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := string(schema.Normalize([]byte(tc.in))); got != tc.want {
				t.Errorf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
