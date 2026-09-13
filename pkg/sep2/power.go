package sep2

// ActivePower represents real power in watts. Value is xs:short (XSD
// Int16, -32768..32767) per sep.xsd; it was previously Go int64, which
// permitted wire values outside the schema's legal range. This is a
// deliberate breaking API change: no compat shim. Downstream callers that
// need a wider intermediate type adapt at their own boundary.
type ActivePower struct {
	Multiplier int8  `xml:"multiplier"`
	Value      int16 `xml:"value"`
}

// ReactivePower represents reactive power in volt-amperes reactive. Value
// is xs:short (XSD Int16, -32768..32767) per sep.xsd; see ActivePower's
// doc comment for the rationale on the int64 to int16 narrowing.
type ReactivePower struct {
	Multiplier int8  `xml:"multiplier"`
	Value      int16 `xml:"value"`
}

// PerCent is a percentage in hundredths of a percent (10000 = 100%).
// IEEE 2030.5-2018 Annex B.2.3.4 "Types package", "PerCent object
// (UInt16)": range 0 to 10000. Unlike ActivePower/ReactivePower, PerCent
// is a schema simple type: it marshals as bare element text, never as
// multiplier+value children.
type PerCent uint16

// SignedPerCent is a signed percentage in hundredths of a percent (10000
// = 100%, -10000 = -100%). IEEE 2030.5-2018 Annex B.2.3.4 "Types
// package", "SignedPerCent object (Int16)": range -10000 to 10000.
type SignedPerCent int16

// FixedPowerFactor for fixed power factor control modes.
type FixedPowerFactor struct {
	Displacement uint16 `xml:"displacement"`
	Excitation   bool   `xml:"excitation"`
	Multiplier   int8   `xml:"multiplier"`
}

// PowerFactor represents a power factor value.
type PowerFactor struct {
	Displacement uint16 `xml:"displacement"`
	Multiplier   int8   `xml:"multiplier"`
}
