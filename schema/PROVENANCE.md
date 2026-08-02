# IEEE 2030.5 schema: provenance and identity

## The file is not here

`sep.xsd` is the normative XML Schema for IEEE 2030.5-2018 (Smart Energy
Profile 2.0). It is copyrighted by IEEE and is **not distributed with this
project**. See the [NOTICE](../NOTICE) file at the repository root for
attribution and for how to obtain a copy at no charge through the IEEE GET
Program.

This document records exactly which copy the module gates against, so a copy
you obtain can be verified before you rely on it.

## Why the module wants it at all

The wire-format gate in `internal/xsdgate` derives its expectations from the
standard itself rather than from hand-transcribed struct tags. It is a
test-only dependency: `package schema` reads the file at test time, and no
binary built from this module contains or reads the schema.

Without a copy, the schema-gated tests skip and the suite stays green. See
`SEP2_SCHEMA_PATH` and `SEP2_SCHEMA_REQUIRED` in NOTICE.

## The document this module gates against

    2030.5-2018 downloads / Model Build 20180301 / sep.xsd

This is the 2018 edition's normative model release. It declares
`targetNamespace="urn:ieee:std:2030.5:ns"`, which is the namespace this
module emits, and it is the copy the EPRI reference client was generated
from. That makes it the right arbiter for interoperability disputes.

Schema attributes:

- `version="2.1.0"`
- `elementFormDefault="qualified"`
- `attributeFormDefault="unqualified"`

## Identity

Copies in circulation differ in line endings and in whether they carry a
UTF-8 BOM, and in nothing else. The authoritative check is therefore taken
over a **normalized** form: the UTF-8 BOM stripped, and every carriage return
removed. `schema.Normalize` performs exactly that transformation, and
`TestSchemaIntegrity` recomputes the digest independently of the loader.

| Measure | Value |
|---|---|
| **Normalized sha256** (BOM and CR stripped) | **`79243a1a01ec4152ce0252af5e5cb89b06019384f79e8cf51f2a5682133e5c9d`** |
| Normalized size | 375001 bytes |
| Line count | 6922 |
| Raw sha256 (original CRLF + BOM copy) | `5b32e18378e64133e3a9f7f320ea10e204bb5596aac96898cdef9e8ec39a9164` |
| Raw md5 (original CRLF + BOM copy) | `e4acb8b31a64a70c06af24f22f2d0441` |
| Raw size (original CRLF + BOM copy) | 381926 bytes |

To verify a copy you have obtained:

    sed -e '1s/^\xef\xbb\xbf//' -e 's/\r//g' sep.xsd | sha256sum

The raw measures are recorded for continuity with earlier releases of this
module, which pinned the byte length and the raw md5 of a vendored copy. They
apply only to a copy that still has its original CRLF line endings and BOM; a
copy that lost either is still the same document and still passes the
normalized check.

A copy whose normalized digest does not match is rejected with a single clear
error naming what was found, rather than being parsed into a wall of
confusing validation failures.

## Do not modify the file

The schema is evidence of what the standard requires. It is never edited,
reformatted, re-indented, ASCII-scrubbed, or "corrected". A locally patched
schema silently stops being evidence, which defeats the point of gating
against it, and the digest check will reject it anyway. If the schema appears
wrong, the finding is recorded here.

## Known upstream erratum (not corrected)

At normalized line 6065, the `xs:documentation` annotation for the PIN
check-digit type cross-references "Section 8.3.2" for the check-digit
calculation. That is stale 2013-edition numbering: in the 2018 edition the
PIN check-digit calculation is at 6.3.5, and 8.3.2 is "List ordering". This
is IEEE's own error, carried in the normative release.

It has no effect on validation, since it lives in an annotation. Anyone
chasing check-digit behaviour should read 6.3.5.

## Related files

- `schema.go`: locates, verifies, and reads an operator-supplied copy.
- `../NOTICE`: attribution and how to obtain the standard.
- `../internal/xsdgate`: the checker that parses this schema and validates
  marshalled output against it. Its doc comment states precisely which XSD
  features it does and does not check.
