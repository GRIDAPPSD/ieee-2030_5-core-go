// Package schema embeds the vendored normative IEEE 2030.5-2018 XML Schema.
//
// The schema is vendored byte-faithful from the standard's Model Build
// 20180301 release and is never edited. See PROVENANCE.md in this directory
// for the source, the integrity hashes, and the one known upstream erratum
// that is deliberately left uncorrected.
//
// Consumers should treat SEP2 as read-only. It is the arbiter of wire-format
// correctness for this module: the gate in internal/xsdgate derives its
// expectations from these bytes rather than from Go struct tags, so a struct
// tag that disagrees with the standard fails the build instead of silently
// producing XML a strict peer rejects.
package schema

import _ "embed"

// SEP2 is the raw bytes of the normative IEEE 2030.5-2018 schema (sep.xsd),
// including its original UTF-8 BOM and CRLF line endings.
//
// Normalised md5 (CR bytes stripped): ea4cce89fadbbf29be78b7d1a8422d08
//
//go:embed sep.xsd
var SEP2 []byte

// Namespace is the XML target namespace declared by SEP2, and the namespace
// this module emits on the wire.
const Namespace = "urn:ieee:std:2030.5:ns"
