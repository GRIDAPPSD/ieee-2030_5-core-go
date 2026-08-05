package xsdgate_test

import (
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/internal/xsdgate"
)

const ns = `xmlns="urn:ieee:std:2030.5:ns"`

// TestValidateCatches is the proof that the gate has teeth. Each case is a
// document that is wrong in exactly one way, drawn from a defect class this
// module has actually shipped. A gate nobody has watched fail is not a gate.
func TestValidateCatches(t *testing.T) {
	s := xsdgate.MustLoad(t)

	tests := []struct {
		name     string
		typeName string
		doc      string
		wantKind xsdgate.ProblemKind
		wantIn   string
	}{
		{
			name:     "element out of sequence order",
			typeName: "Reading",
			// Schema order is consumptionBlock, qualityFlags, timePeriod,
			// touTier, value, localID. Here value precedes qualityFlags.
			doc: `<Reading ` + ns + `>
				<value>42</value>
				<qualityFlags>0A0B</qualityFlags>
			</Reading>`,
			wantKind: xsdgate.KindOrder,
			wantIn:   "qualityFlags",
		},
		{
			name:     "required element dropped by omitempty",
			typeName: "Registration",
			// pIN is minOccurs=1 and is absent.
			doc: `<Registration ` + ns + `>
				<dateTimeRegistered>1500000000</dateTimeRegistered>
			</Registration>`,
			wantKind: xsdgate.KindMissingElement,
			wantIn:   "pIN",
		},
		{
			name:     "element the schema does not declare",
			typeName: "ReadingType",
			// ReadingType has no mRID anywhere in the schema.
			doc: `<ReadingType ` + ns + `>
				<mRID>1234567890ABCDEF</mRID>
			</ReadingType>`,
			wantKind: xsdgate.KindUnknownElement,
			wantIn:   "mRID",
		},
		{
			name:     "schema attribute emitted as a child element",
			typeName: "Registration",
			// pollRate is an ATTRIBUTE on Registration.
			doc: `<Registration ` + ns + `>
				<dateTimeRegistered>1500000000</dateTimeRegistered>
				<pIN>123456</pIN>
				<pollRate>900</pollRate>
			</Registration>`,
			wantKind: xsdgate.KindPlacement,
			wantIn:   "ATTRIBUTE",
		},
		{
			name:     "schema element emitted as an attribute",
			typeName: "Reading",
			// value is a child ELEMENT, not an attribute.
			doc:      `<Reading ` + ns + ` value="42"></Reading>`,
			wantKind: xsdgate.KindPlacement,
			wantIn:   "CHILD ELEMENT",
		},
		{
			name:     "decimal emitted where hexBinary is required",
			typeName: "Reading",
			// qualityFlags is HexBinary16. Decimal 5 renders as "5", an odd
			// digit count, which cannot be a whole number of octets.
			doc: `<Reading ` + ns + `>
				<qualityFlags>5</qualityFlags>
			</Reading>`,
			wantKind: xsdgate.KindLexical,
			wantIn:   "odd digit count",
		},
		{
			name:     "hexBinary exceeding its maxLength facet",
			typeName: "Reading",
			// HexBinary16 permits 2 octets; this is 3.
			doc: `<Reading ` + ns + `>
				<qualityFlags>AABBCC</qualityFlags>
			</Reading>`,
			wantKind: xsdgate.KindLexical,
			wantIn:   "maxLength",
		},
		{
			name:     "hexBinary containing a non-hex character",
			typeName: "Reading",
			doc: `<Reading ` + ns + `>
				<qualityFlags>ZZ</qualityFlags>
			</Reading>`,
			wantKind: xsdgate.KindLexical,
			wantIn:   "non-hex character",
		},
		{
			name:     "integer outside its type range",
			typeName: "ReadingType",
			// maxNumberOfIntervals is UInt8.
			doc: `<ReadingType ` + ns + `>
				<maxNumberOfIntervals>9999</maxNumberOfIntervals>
			</ReadingType>`,
			wantKind: xsdgate.KindLexical,
			wantIn:   "exceeds max",
		},
		{
			name:     "maxOccurs=1 element repeated",
			typeName: "Reading",
			doc: `<Reading ` + ns + `>
				<value>1</value>
				<value>2</value>
			</Reading>`,
			wantKind: xsdgate.KindRepetition,
			wantIn:   "appeared 2 times",
		},
		{
			name:     "unknown attribute",
			typeName: "Reading",
			doc:      `<Reading ` + ns + ` bogusAttr="x"></Reading>`,
			wantKind: xsdgate.KindUnknownAttribute,
			wantIn:   "bogusAttr",
		},
		{
			// The root-level schemaVer tolerance is narrow: it applies to
			// the root element only. A schemaVer attribute
			// emitted on a non-root, nested complex element is still not
			// declared by the schema and is still an unknown attribute
			// there.
			name:     "schemaVer attribute on a nested, non-root element is still unknown",
			typeName: "MirrorUsagePoint",
			doc: `<MirrorUsagePoint ` + ns + `>
				<mRID>0102030405060708090A0B0C0D0E0F10</mRID>
				<roleFlags>0013</roleFlags>
				<serviceCategoryKind>0</serviceCategoryKind>
				<status>1</status>
				<deviceLFDI>0102030405060708090A0B0C0D0E0F1011121314</deviceLFDI>
				<MirrorMeterReading schemaVer="2.2"><mRID>0102030405060708090A0B0C0D0E0F11</mRID></MirrorMeterReading>
			</MirrorUsagePoint>`,
			wantKind: xsdgate.KindUnknownAttribute,
			wantIn:   "schemaVer",
		},
		{
			name:     "wrong root namespace",
			typeName: "Reading",
			// The 2013 namespace, which a 2018 peer rejects.
			doc:      `<Reading xmlns="http://ieee.org/2030.5"></Reading>`,
			wantKind: xsdgate.KindNamespace,
			wantIn:   "targetNamespace",
		},
		{
			name:     "defect nested inside a child element",
			typeName: "UsagePoint",
			// deviceLFDI is HexBinary160 and "7" has an odd digit count.
			// This proves recursion reaches beyond the root.
			doc: `<UsagePoint ` + ns + `>
				<mRID>0102030405060708090A0B0C0D0E0F10</mRID>
				<roleFlags>0013</roleFlags>
				<serviceCategoryKind>0</serviceCategoryKind>
				<status>1</status>
				<deviceLFDI>7</deviceLFDI>
			</UsagePoint>`,
			wantKind: xsdgate.KindLexical,
			wantIn:   "odd digit count",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			problems, err := s.Validate(tc.typeName, []byte(tc.doc))
			if err != nil {
				t.Fatalf("validate: %v", err)
			}
			if !problems.Has(tc.wantKind) {
				t.Fatalf("gate did not report kind %q; got:\n%s", tc.wantKind, problems.Error())
			}
			if !strings.Contains(problems.Error(), tc.wantIn) {
				t.Errorf("problem text lacks %q; got:\n%s", tc.wantIn, problems.Error())
			}
		})
	}
}

// TestValidateAcceptsConformantDocuments guards the other direction. A gate
// that fails everything is as useless as one that fails nothing, and false
// positives are what get a gate switched off.
func TestValidateAcceptsConformantDocuments(t *testing.T) {
	s := xsdgate.MustLoad(t)

	tests := []struct {
		name     string
		typeName string
		doc      string
	}{
		{
			name:     "all optional elements omitted",
			typeName: "Reading",
			doc:      `<Reading ` + ns + `></Reading>`,
		},
		{
			name:     "full sequence in schema order",
			typeName: "Reading",
			doc: `<Reading ` + ns + ` href="/upt/0/mr/0/r/1">
				<consumptionBlock>0</consumptionBlock>
				<qualityFlags>0A0B</qualityFlags>
				<touTier>0</touTier>
				<value>42</value>
				<localID>00FF</localID>
			</Reading>`,
		},
		{
			name:     "required elements present in order",
			typeName: "Registration",
			doc: `<Registration ` + ns + ` pollRate="900">
				<dateTimeRegistered>1500000000</dateTimeRegistered>
				<pIN>123456</pIN>
			</Registration>`,
		},
		{
			name:     "inherited particles precede own particles",
			typeName: "MirrorUsagePoint",
			doc: `<MirrorUsagePoint ` + ns + `>
				<mRID>0102030405060708090A0B0C0D0E0F10</mRID>
				<description>meter</description>
				<roleFlags>0013</roleFlags>
				<serviceCategoryKind>0</serviceCategoryKind>
				<status>1</status>
				<deviceLFDI>0102030405060708090A0B0C0D0E0F1011121314</deviceLFDI>
			</MirrorUsagePoint>`,
		},
		{
			name:     "repeatable element may appear many times",
			typeName: "MirrorUsagePoint",
			doc: `<MirrorUsagePoint ` + ns + `>
				<mRID>0102030405060708090A0B0C0D0E0F10</mRID>
				<roleFlags>0013</roleFlags>
				<serviceCategoryKind>0</serviceCategoryKind>
				<status>1</status>
				<deviceLFDI>0102030405060708090A0B0C0D0E0F1011121314</deviceLFDI>
				<MirrorMeterReading><mRID>0102030405060708090A0B0C0D0E0F11</mRID></MirrorMeterReading>
				<MirrorMeterReading><mRID>0102030405060708090A0B0C0D0E0F12</mRID></MirrorMeterReading>
			</MirrorUsagePoint>`,
		},
		{
			// IEEE 2030.5-2023 clause 5.6.2 REQUIRES schemaVer on the root
			// element of every payload. The 2.1 schema this gate checks
			// against predates that requirement and declares no such
			// attribute, so a conformant 2023 peer must not be rejected for
			// carrying it.
			name:     "2023 schemaVer attribute on the root element",
			typeName: "Reading",
			doc:      `<Reading ` + ns + ` schemaVer="2.2"></Reading>`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			problems, err := s.Validate(tc.typeName, []byte(tc.doc))
			if err != nil {
				t.Fatalf("validate: %v", err)
			}
			if len(problems) != 0 {
				t.Errorf("conformant document reported %d problem(s):\n%s", len(problems), problems.Error())
			}
		})
	}
}

// TestHexBinaryEvenLengthDecimalIsNotCaught documents a real limitation as an
// executable fact rather than a comment nobody reads. "16" is a valid
// hexBinary denoting 0x16, so a decimal 16 emitted where hex is required is
// lexically indistinguishable from correct output. Anyone relying on the gate
// for hex correctness needs to know this.
func TestHexBinaryEvenLengthDecimalIsNotCaught(t *testing.T) {
	s := xsdgate.MustLoad(t)

	doc := `<Reading ` + ns + `><qualityFlags>16</qualityFlags></Reading>`
	problems, err := s.Validate("Reading", []byte(doc))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(problems) != 0 {
		t.Fatalf("expected the gate to accept %q as valid hexBinary, got:\n%s", "16", problems.Error())
	}
	t.Log("documented limitation: an even-length all-digit decimal passes the hexBinary check, " +
		"because it is a syntactically valid hexBinary with a different value. " +
		"Value-level assertions are required where that collision matters.")
}

// TestValidateRejectsUnknownType makes the failure mode explicit rather than
// silently passing when a caller typos a type name. A gate that quietly
// validates nothing is the worst outcome available.
func TestValidateRejectsUnknownType(t *testing.T) {
	s := xsdgate.MustLoad(t)

	_, err := s.Validate("NoSuchType", []byte(`<NoSuchType `+ns+`/>`))
	if err == nil {
		t.Fatal("expected an error for a type absent from the schema")
	}
	if !strings.Contains(err.Error(), "not a complexType") {
		t.Errorf("error should name the cause, got: %v", err)
	}
}
