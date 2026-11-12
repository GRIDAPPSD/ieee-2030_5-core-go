package sep2

// The five demand response value types below are the children shared by
// EndDeviceControl and DrResponse in sep.xsd. Element order is the schema's
// sequence order.

// ApplianceLoadReduction asks an ENERGY STAR appliance for a load reduction.
// Type is ApplianceLoadReductionType (UInt8).
type ApplianceLoadReduction struct {
	Type uint8 `xml:"type"`
}

// Copy returns an independent copy.
func (a ApplianceLoadReduction) Copy() ApplianceLoadReduction { return a }

// DutyCycle carries the maximum On-state duty cycle as a percentage of time.
type DutyCycle struct {
	NormalValue uint8 `xml:"normalValue"`
}

// Copy returns an independent copy.
func (d DutyCycle) Copy() DutyCycle { return d }

// Offset is a requested change to the normal temperature set points
// (degrees C / 10) or load adjustment percentage. A nil field is absent.
type Offset struct {
	CoolingOffset                  *uint8   `xml:"coolingOffset,omitempty"`
	HeatingOffset                  *uint8   `xml:"heatingOffset,omitempty"`
	LoadAdjustmentPercentageOffset *PerCent `xml:"loadAdjustmentPercentageOffset,omitempty"`
}

// Copy returns an independent copy.
func (o Offset) Copy() Offset {
	c := o
	c.CoolingOffset = clonePtr(o.CoolingOffset)
	c.HeatingOffset = clonePtr(o.HeatingOffset)
	c.LoadAdjustmentPercentageOffset = clonePtr(o.LoadAdjustmentPercentageOffset)
	return c
}

// SetPoint holds temperature set points in degrees C / 100. A nil field is
// absent.
type SetPoint struct {
	CoolingSetpoint *int16 `xml:"coolingSetpoint,omitempty"`
	HeatingSetpoint *int16 `xml:"heatingSetpoint,omitempty"`
}

// Copy returns an independent copy.
func (s SetPoint) Copy() SetPoint {
	c := s
	c.CoolingSetpoint = clonePtr(s.CoolingSetpoint)
	c.HeatingSetpoint = clonePtr(s.HeatingSetpoint)
	return c
}

// TargetReduction is a recommended consumption threshold. Type is UnitType
// (UInt8); Value is UInt16.
type TargetReduction struct {
	Type  uint8  `xml:"type"`
	Value uint16 `xml:"value"`
}

// Copy returns an independent copy.
func (t TargetReduction) Copy() TargetReduction { return t }
