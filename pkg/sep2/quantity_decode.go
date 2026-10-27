package sep2

import (
	"bytes"
	"encoding/xml"
	"fmt"
)

// decodeQuantity decodes a {multiplier, value} element into dst (a pointer
// to a method-free copy of the quantity type) and refuses character data
// directly inside the element. The default decoder reads a bare integer
// such as <rtgMaxA>16</rtgMaxA> as {0, 0} with no error, which turns a peer's
// pre-complex-type rating into a silent zero. An empty element and the
// complex form decode as before.
func decodeQuantity(d *xml.Decoder, start xml.StartElement, typeName string, dst any) error {
	g := &quantityTokens{d: d, pending: &start}
	if err := xml.NewTokenDecoder(g).Decode(dst); err != nil {
		return fmt.Errorf("sep2: %s: %w", typeName, err)
	}
	if g.text != "" {
		return fmt.Errorf("sep2: %s: element <%s> has text %q, want <multiplier> and <value> children", typeName, start.Name.Local, g.text)
	}
	return nil
}

// quantityTokens replays the start element, then forwards d's tokens while
// recording the first non-blank character data seen at the element's own depth.
type quantityTokens struct {
	d       *xml.Decoder
	pending *xml.StartElement
	depth   int
	text    string
}

func (g *quantityTokens) Token() (xml.Token, error) {
	if g.pending != nil {
		t := *g.pending
		g.pending = nil
		g.depth = 1
		return t, nil
	}
	tok, err := g.d.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case xml.StartElement:
		g.depth++
	case xml.EndElement:
		g.depth--
	case xml.CharData:
		if g.depth == 1 && g.text == "" {
			g.text = string(bytes.TrimSpace(t))
		}
	}
	return tok, nil
}

// UnmarshalXML refuses a bare-integer element instead of reading it as zero.
func (q *ActivePower) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	type plain ActivePower
	return decodeQuantity(d, start, "ActivePower", (*plain)(q))
}

// UnmarshalXML refuses a bare-integer element instead of reading it as zero.
func (q *ReactivePower) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	type plain ReactivePower
	return decodeQuantity(d, start, "ReactivePower", (*plain)(q))
}

// UnmarshalXML refuses a bare-integer element instead of reading it as zero.
func (q *ApparentPower) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	type plain ApparentPower
	return decodeQuantity(d, start, "ApparentPower", (*plain)(q))
}

// UnmarshalXML refuses a bare-integer element instead of reading it as zero.
func (q *VoltageRMS) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	type plain VoltageRMS
	return decodeQuantity(d, start, "VoltageRMS", (*plain)(q))
}

// UnmarshalXML refuses a bare-integer element instead of reading it as zero.
func (q *CurrentRMS) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	type plain CurrentRMS
	return decodeQuantity(d, start, "CurrentRMS", (*plain)(q))
}

// UnmarshalXML refuses a bare-integer element instead of reading it as zero.
func (q *AmpereHour) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	type plain AmpereHour
	return decodeQuantity(d, start, "AmpereHour", (*plain)(q))
}

// UnmarshalXML refuses a bare-integer element instead of reading it as zero.
func (q *WattHour) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	type plain WattHour
	return decodeQuantity(d, start, "WattHour", (*plain)(q))
}

// UnmarshalXML refuses a bare-integer element instead of reading it as zero.
func (q *ReactiveSusceptance) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	type plain ReactiveSusceptance
	return decodeQuantity(d, start, "ReactiveSusceptance", (*plain)(q))
}

// UnmarshalXML refuses a bare-integer element instead of reading it as zero.
func (q *RealEnergy) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	type plain RealEnergy
	return decodeQuantity(d, start, "RealEnergy", (*plain)(q))
}

// UnmarshalXML refuses a bare-integer element instead of reading it as zero.
func (q *SignedRealEnergy) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	type plain SignedRealEnergy
	return decodeQuantity(d, start, "SignedRealEnergy", (*plain)(q))
}

// UnmarshalXML refuses a bare-integer element instead of reading it as zero.
func (q *Temperature) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	type plain Temperature
	return decodeQuantity(d, start, "Temperature", (*plain)(q))
}
