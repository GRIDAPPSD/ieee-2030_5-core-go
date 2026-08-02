// Package schema locates the normative IEEE 2030.5 XML Schema (sep.xsd).
//
// The schema is copyrighted by IEEE and is NOT distributed with this
// project. See the NOTICE file at the repository root for attribution and
// for how to obtain a copy at no charge through the IEEE GET Program.
//
// Load reads an operator-supplied copy at run time. Its only consumer is
// the wire-format gate in internal/xsdgate, which is test-only: nothing
// outside a _test.go file reads the schema, and no binary built from this
// module contains it.
//
// Absence is not failure. When no copy is configured, Load returns an error
// wrapping ErrNotFound and the schema-gated tests skip, so a contributor
// without an IEEE copy still runs the suite green. Absence is also never
// silently satisfiable: set SEP2_SCHEMA_REQUIRED (see Required) and a
// missing copy becomes a hard failure instead.
package schema

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// EnvPath names the environment variable holding the path to a copy of
	// sep.xsd. It takes precedence over DefaultPath.
	EnvPath = "SEP2_SCHEMA_PATH"

	// EnvRequired names the environment variable that converts an absent
	// schema from a skip into a hard failure. See Required.
	EnvRequired = "SEP2_SCHEMA_REQUIRED"

	// DefaultPath is the conventional location for a locally supplied copy,
	// relative to the module root. It is gitignored, so a developer can drop
	// a licensed copy there and have the gate work with no configuration and
	// no risk of committing the file.
	DefaultPath = "schema/sep.xsd"

	// NormalizedSHA256 identifies the exact document this module gates
	// against: IEEE 2030.5-2018, Model Build 20180301, sep.xsd. The digest is
	// taken over the normalized form (see Normalize) so that copies differing
	// only in line endings or in the presence of a UTF-8 BOM still match; the
	// copies in circulation differ in exactly those two ways.
	NormalizedSHA256 = "79243a1a01ec4152ce0252af5e5cb89b06019384f79e8cf51f2a5682133e5c9d"

	// NormalizedSize is the length in bytes of that same normalized form. It
	// is reported alongside a digest mismatch so a wrong-document error is
	// readable rather than just two unequal hex strings.
	NormalizedSize = 375001
)

// Namespace is the XML target namespace declared by sep.xsd, and the
// namespace this module emits on the wire. It is a fact about the standard,
// not about any particular copy of the file, so it stays available whether
// or not a schema copy is present.
const Namespace = "urn:ieee:std:2030.5:ns"

// ErrNotFound reports that no copy of the schema is configured or present.
// Callers distinguish it with errors.Is and skip rather than fail: it means
// "not available here", not "wrong" or "broken".
var ErrNotFound = errors.New("IEEE 2030.5 schema (sep.xsd) not available: it is not distributed with this project; " +
	"set " + EnvPath + " to a copy, or place one at " + DefaultPath + " under the module root; " +
	"see the NOTICE file for how to obtain it at no charge from IEEE")

// IntegrityError reports that the file found is not the document this module
// gates against. It is deliberately NOT an ErrNotFound: validating against
// the wrong schema is worse than not validating at all, so it fails loudly
// instead of skipping.
type IntegrityError struct {
	// Path is the file that was read.
	Path string
	// SHA256 is the normalized digest actually computed.
	SHA256 string
	// Size is the normalized length actually read, in bytes.
	Size int
}

func (e *IntegrityError) Error() string {
	return fmt.Sprintf("%s is not the IEEE 2030.5 schema this module gates against: "+
		"normalized sha256 %s (%d bytes), want %s (%d bytes); "+
		"the expected document is IEEE 2030.5-2018 Model Build 20180301 sep.xsd (see schema/PROVENANCE.md); "+
		"point %s at that document. The schema-gated tests skip only when no copy is found at all, "+
		"never when the copy found is the wrong document",
		e.Path, e.SHA256, e.Size, NormalizedSHA256, NormalizedSize, EnvPath)
}

// Required reports whether the caller has demanded that a schema copy be
// present, by setting EnvRequired to a truthy value.
//
// This is the switch that keeps absence from being silently satisfiable. CI
// that supplies a schema sets it, so a rotated secret, a typo in the path
// variable, or a decode step that quietly produced nothing fails the run
// instead of skipping every gated test and reporting green.
//
// An unparseable value is an error rather than a shrug: a run configured
// with SEP2_SCHEMA_REQUIRED=ture must not disarm the gate.
func Required() (bool, error) {
	raw := strings.TrimSpace(os.Getenv(EnvRequired))
	if raw == "" {
		return false, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s=%q is not a boolean: use 1 or 0", EnvRequired, raw)
	}
	return v, nil
}

// Load returns the schema bytes and the path they were read from.
//
// Resolution order is EnvPath, then DefaultPath under the module root. A
// missing copy returns an error wrapping ErrNotFound. A copy that is not the
// expected document returns *IntegrityError. An EnvPath that is set but does
// not resolve returns neither: an explicitly configured path that is wrong is
// a misconfiguration, and reporting it as "not available" would let a broken
// CI secret masquerade as an absent schema.
func Load() (data []byte, path string, err error) {
	path, err = Resolve()
	if err != nil {
		return nil, "", err
	}
	data, err = os.ReadFile(path)
	if err != nil {
		return nil, path, fmt.Errorf("reading IEEE 2030.5 schema %s: %w", path, err)
	}
	norm := Normalize(data)
	sum := sha256.Sum256(norm)
	if got := hex.EncodeToString(sum[:]); got != NormalizedSHA256 {
		return nil, path, &IntegrityError{Path: path, SHA256: got, Size: len(norm)}
	}
	return data, path, nil
}

// Resolve returns the path of the schema copy to read, without reading it.
func Resolve() (string, error) {
	if p := strings.TrimSpace(os.Getenv(EnvPath)); p != "" {
		if err := checkFile(p); err != nil {
			return "", fmt.Errorf("%s=%q: %w", EnvPath, p, err)
		}
		return p, nil
	}
	root, err := moduleRoot()
	if err != nil {
		return "", fmt.Errorf("%w (%v)", ErrNotFound, err)
	}
	p := filepath.Join(root, DefaultPath)
	if err := checkFile(p); err != nil {
		return "", fmt.Errorf("%w (looked for %s)", ErrNotFound, p)
	}
	return p, nil
}

// Normalize strips the UTF-8 BOM and every carriage return, yielding the
// form NormalizedSHA256 and NormalizedSize describe. Copies of sep.xsd in
// circulation are byte-identical apart from those two carriers, so
// normalizing is what makes the identity check portable rather than a
// false alarm on a checkout that converted line endings.
func Normalize(data []byte) []byte {
	out := bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	return bytes.ReplaceAll(out, []byte("\r"), nil)
}

func checkFile(p string) error {
	info, err := os.Stat(p)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory, want a file", p)
	}
	return nil
}

// moduleRoot walks up from the working directory to the directory holding
// go.mod. Go tests run with the working directory set to their own package
// directory, so DefaultPath cannot be resolved relative to it directly.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no go.mod found above the working directory")
		}
		dir = parent
	}
}
