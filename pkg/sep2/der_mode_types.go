package sep2

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// DERCurveLink references a DERCurve resource by URI. IEEE 2030.5-2018
// sep.xsd DERCurveLink, an extension of Link: an empty element whose
// required href attribute is the curve's URI.
type DERCurveLink struct {
	Href string `xml:"href,attr"`
}

var errEmptyCurveHref = errors.New("sep2: DERCurveLink: href is required")

// MarshalXML refuses an empty href, which the schema requires.
func (l DERCurveLink) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	if strings.TrimSpace(l.Href) == "" {
		return errEmptyCurveHref
	}
	start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: "href"}, Value: l.Href})
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	return e.EncodeToken(start.End())
}

// UnmarshalXML requires the href and refuses any content, so the pre-#153
// integer form (<opModVoltVar>3</opModVoltVar>) errors instead of decoding
// as a link with no href.
func (l *DERCurveLink) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var href string
	for _, a := range start.Attr {
		if a.Name.Space == "" && a.Name.Local == "href" {
			href = a.Value
		}
	}
	if strings.TrimSpace(href) == "" {
		return errEmptyCurveHref
	}
	if err := decodeChildren(d, "DERCurveLink", nil, nil); err != nil {
		return err
	}
	l.Href = href
	return nil
}

// MarshalJSON mirrors MarshalXML's href guard.
func (l DERCurveLink) MarshalJSON() ([]byte, error) {
	if strings.TrimSpace(l.Href) == "" {
		return nil, errEmptyCurveHref
	}
	type plain DERCurveLink
	return json.Marshal(plain(l))
}

// UnmarshalJSON applies the XML rules to the JSON server-go persists, so a
// stored integer curve ref fails to load.
func (l *DERCurveLink) UnmarshalJSON(data []byte) error {
	var raw struct{ Href *string }
	if err := decodeJSONStrict(data, &raw); err != nil {
		return fmt.Errorf("sep2: DERCurveLink: %w", err)
	}
	if raw.Href == nil || strings.TrimSpace(*raw.Href) == "" {
		return errEmptyCurveHref
	}
	l.Href = *raw.Href
	return nil
}

// FixedVar is the opModFixedVar setting: reactive power as a signed
// percentage, in hundredths of a percent, of the quantity RefType names.
// IEEE 2030.5-2018 sep.xsd FixedVar; RefType is a DERUnitRefType (UInt8).
type FixedVar struct {
	RefType uint8         `xml:"refType"`
	Value   SignedPerCent `xml:"value"`
}

// UnmarshalXML requires refType then value. The default decoder would read
// the pre-#153 ReactivePower form (multiplier, value) as refType 0.
func (f *FixedVar) UnmarshalXML(d *xml.Decoder, _ xml.StartElement) error {
	var v FixedVar
	err := decodeChildren(d, "FixedVar", []string{"refType", "value"}, func(name, text string) error {
		if name == "refType" {
			n, err := parseUint(text, 8)
			v.RefType = uint8(n)
			return err
		}
		var err error
		v.Value, err = parseSignedPerCent(text)
		return err
	})
	if err != nil {
		return err
	}
	*f = v
	return nil
}

// UnmarshalJSON requires both fields and refuses unknown ones, for the same
// reason as UnmarshalXML.
func (f *FixedVar) UnmarshalJSON(data []byte) error {
	var raw struct {
		RefType *uint8
		Value   *SignedPerCent
	}
	if err := decodeJSONStrict(data, &raw); err != nil {
		return fmt.Errorf("sep2: FixedVar: %w", err)
	}
	if raw.RefType == nil || raw.Value == nil {
		return errors.New("sep2: FixedVar: RefType and Value are required")
	}
	*f = FixedVar{RefType: *raw.RefType, Value: *raw.Value}
	return nil
}

// FreqDroopType is the opModFreqDroop setting. IEEE 2030.5-2018 sep.xsd
// FreqDroopType; every element is required.
type FreqDroopType struct {
	DBOF        uint32 `xml:"dBOF"`
	DBUF        uint32 `xml:"dBUF"`
	KOF         uint16 `xml:"kOF"`
	KUF         uint16 `xml:"kUF"`
	OpenLoopTms uint16 `xml:"openLoopTms"`
}

// UnmarshalXML requires all five elements in schema order. The default
// decoder would read the pre-#153 scalar form as all zeros.
func (f *FreqDroopType) UnmarshalXML(d *xml.Decoder, _ xml.StartElement) error {
	var v FreqDroopType
	err := decodeChildren(d, "FreqDroopType", []string{"dBOF", "dBUF", "kOF", "kUF", "openLoopTms"}, func(name, text string) error {
		var err error
		var n uint64
		switch name {
		case "dBOF", "dBUF":
			n, err = parseUint(text, 32)
		default:
			n, err = parseUint(text, 16)
		}
		switch name {
		case "dBOF":
			v.DBOF = uint32(n)
		case "dBUF":
			v.DBUF = uint32(n)
		case "kOF":
			v.KOF = uint16(n)
		case "kUF":
			v.KUF = uint16(n)
		case "openLoopTms":
			v.OpenLoopTms = uint16(n)
		}
		return err
	})
	if err != nil {
		return err
	}
	*f = v
	return nil
}

// UnmarshalJSON requires all five fields and refuses unknown ones.
func (f *FreqDroopType) UnmarshalJSON(data []byte) error {
	var raw struct {
		DBOF, DBUF            *uint32
		KOF, KUF, OpenLoopTms *uint16
	}
	if err := decodeJSONStrict(data, &raw); err != nil {
		return fmt.Errorf("sep2: FreqDroopType: %w", err)
	}
	if raw.DBOF == nil || raw.DBUF == nil || raw.KOF == nil || raw.KUF == nil || raw.OpenLoopTms == nil {
		return errors.New("sep2: FreqDroopType: DBOF, DBUF, KOF, KUF and OpenLoopTms are required")
	}
	*f = FreqDroopType{DBOF: *raw.DBOF, DBUF: *raw.DBUF, KOF: *raw.KOF, KUF: *raw.KUF, OpenLoopTms: *raw.OpenLoopTms}
	return nil
}

// decodeChildren consumes the rest of an element whose content must be
// exactly the simple-typed children in want, in order, each once, with no
// other text. set receives each child's text.
func decodeChildren(d *xml.Decoder, typeName string, want []string, set func(name, text string) error) error {
	next := 0
	for {
		tok, err := d.Token()
		if err != nil {
			return fmt.Errorf("sep2: %s: %w", typeName, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if next >= len(want) || t.Name.Local != want[next] {
				return fmt.Errorf("sep2: %s: unexpected element <%s>", typeName, t.Name.Local)
			}
			var text string
			if err := d.DecodeElement(&text, &t); err != nil {
				return fmt.Errorf("sep2: %s.%s: %w", typeName, t.Name.Local, err)
			}
			if err := set(t.Name.Local, text); err != nil {
				return fmt.Errorf("sep2: %s.%s: %w", typeName, t.Name.Local, err)
			}
			next++
		case xml.CharData:
			if len(bytes.TrimSpace(t)) > 0 {
				return fmt.Errorf("sep2: %s: unexpected text %q", typeName, bytes.TrimSpace(t))
			}
		case xml.EndElement:
			if next < len(want) {
				return fmt.Errorf("sep2: %s: missing element <%s>", typeName, want[next])
			}
			return nil
		}
	}
}

func parseUint(text string, bits int) (uint64, error) {
	return strconv.ParseUint(strings.TrimSpace(text), 10, bits)
}

func decodeJSONStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
