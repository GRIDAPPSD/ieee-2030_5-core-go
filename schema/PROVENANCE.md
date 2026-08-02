# Vendored IEEE 2030.5 schema: provenance

## File

`sep.xsd` is the normative XML Schema for IEEE 2030.5-2018 (Smart Energy
Profile 2.0). It is vendored here verbatim so the wire-format gate in
`internal/xsdgate` derives its expectations from the standard itself rather
than from hand-transcribed struct tags.

## Source

    2030.5-2018 downloads / Model Build 20180301 / sep.xsd

This is the 2018 edition's normative model release. It declares
`targetNamespace="urn:ieee:std:2030.5:ns"`, which is the namespace this
module emits, and it is the copy the EPRI reference client was generated
from. That makes it the right arbiter for interoperability disputes.

Schema attributes as vendored:

- `version="2.1.0"`
- `elementFormDefault="qualified"`
- `attributeFormDefault="unqualified"`

## Integrity

| Measure | Value |
|---|---|
| Size | 381926 bytes |
| Line count | 6922 |
| Raw md5 (as vendored, CRLF + BOM) | `e4acb8b31a64a70c06af24f22f2d0441` |
| **Normalised md5** (CR stripped) | **`ea4cce89fadbbf29be78b7d1a8422d08`** |

The **normalised** md5 is the authoritative identity check. Copies of this
file in circulation differ only in line endings, so a raw md5 comparison
reports a false mismatch against an LF copy. To verify:

    tr -d '\r' < schema/sep.xsd | md5sum

The vendored file retains its original UTF-8 BOM and CRLF line endings.
`.gitattributes` marks it `-text` so git does not normalise them on
checkout or commit.

## Do not modify this file

The schema is vendored byte-faithful and is never edited, reformatted,
re-indented, ASCII-scrubbed, or "corrected". A locally patched schema
silently stops being evidence of what the standard requires, which defeats
the entire purpose of gating against it. If the schema appears wrong, the
finding is recorded here, not applied to the file.

### ASCII policy exemption

The workspace ASCII rule does not apply to this file. It contains 195
non-ASCII bytes: a UTF-8 BOM, typographic quotes, en-dashes, and degree and
registered-trademark signs, all inside `xs:documentation` prose. They are
upstream bytes and are preserved. No Go source, comment, or note in this
change carries non-ASCII characters.

## Known upstream erratum (not corrected here)

At normalised line 6065, the documentation for the PIN check-digit type
reads:

> Unsigned integer, max inclusive 687194767359, which is 2^36-1
> (68719476735), with added check digit. See Section 8.3.2 for check digit
> calculation.

The cross-reference "Section 8.3.2" is stale 2013-edition numbering. In the
2018 edition the PIN check-digit calculation is at 6.3.5, and 8.3.2 is
"List ordering". This is IEEE's own error carried in the normative release,
not damage introduced by vendoring.

It is recorded rather than fixed, for two reasons: it lives in an
annotation and has no effect on validation, and a modified schema is worse
than a wrong annotation. Anyone chasing check-digit behaviour should read
6.3.5.

## Related files

- `schema.go`: embeds `sep.xsd` and exposes it to the module.
- `../internal/xsdgate`: the checker that parses this schema and validates
  marshalled output against it. Its doc comment states precisely which XSD
  features it does and does not check.
