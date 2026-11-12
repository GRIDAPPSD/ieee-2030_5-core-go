package sep2

import "encoding/xml"

// DemandResponseProgram is a demand response program.
// Spec reference: section 10.3 (stub: handlers deferred)
type DemandResponseProgram struct {
	XMLName xml.Name `xml:"urn:ieee:std:2030.5:ns DemandResponseProgram"`
	SubscribableResource
	IdentifiedObject
	Primacy                        uint8     `xml:"primacy"`
	ActiveEndDeviceControlListLink *ListLink `xml:"ActiveEndDeviceControlListLink,omitempty"`
	EndDeviceControlListLink       *ListLink `xml:"EndDeviceControlListLink,omitempty"`
}

// Copy returns an independent copy.
func (d DemandResponseProgram) Copy() DemandResponseProgram {
	c := d
	c.SubscribableResource = d.SubscribableResource.Copy()
	c.IdentifiedObject = d.IdentifiedObject.Copy()
	c.ActiveEndDeviceControlListLink = clonePtr(d.ActiveEndDeviceControlListLink)
	c.EndDeviceControlListLink = clonePtr(d.EndDeviceControlListLink)
	return c
}

// DemandResponseProgramList is a list of DemandResponseProgram resources.
type DemandResponseProgramList struct {
	XMLName xml.Name `xml:"urn:ieee:std:2030.5:ns DemandResponseProgramList"`
	ListResource
	DemandResponseProgram []DemandResponseProgram `xml:"DemandResponseProgram,omitempty"`
}

// EndDeviceControl is a demand response control event. Fields follow the
// sep.xsd sequence. The two booleans are required and always encoded; a body
// that omits one decodes to false.
//
// DeviceCategory is required by the schema but stays a pointer: a value type
// would encode an invented 00000000 for a control that never set it, so the
// consumer checks presence instead.
type EndDeviceControl struct {
	XMLName xml.Name `xml:"urn:ieee:std:2030.5:ns EndDeviceControl"`
	RandomizableEvent
	ApplianceLoadReduction *ApplianceLoadReduction `xml:"ApplianceLoadReduction,omitempty"`
	DeviceCategory         *DeviceCategoryType     `xml:"deviceCategory,omitempty"`
	DRProgramMandatory     bool                    `xml:"drProgramMandatory"`
	DutyCycle              *DutyCycle              `xml:"DutyCycle,omitempty"`
	LoadShiftForward       bool                    `xml:"loadShiftForward"`
	Offset                 *Offset                 `xml:"Offset,omitempty"`
	OverrideDuration       *uint16                 `xml:"overrideDuration,omitempty"`
	SetPoint               *SetPoint               `xml:"SetPoint,omitempty"`
	TargetReduction        *TargetReduction        `xml:"TargetReduction,omitempty"`
}

// Copy returns an independent copy.
func (e EndDeviceControl) Copy() EndDeviceControl {
	c := e
	c.Event = e.Event.Copy()
	c.DeviceCategory = clonePtr(e.DeviceCategory)
	c.RandomizeDuration = clonePtr(e.RandomizeDuration)
	c.RandomizeStart = clonePtr(e.RandomizeStart)
	c.ApplianceLoadReduction = clonePtr(e.ApplianceLoadReduction)
	c.DutyCycle = clonePtr(e.DutyCycle)
	c.Offset = cloneCopyPtr(e.Offset)
	c.OverrideDuration = clonePtr(e.OverrideDuration)
	c.SetPoint = cloneCopyPtr(e.SetPoint)
	c.TargetReduction = clonePtr(e.TargetReduction)
	return c
}

// EndDeviceControlList is a list of EndDeviceControl resources.
type EndDeviceControlList struct {
	XMLName xml.Name `xml:"urn:ieee:std:2030.5:ns EndDeviceControlList"`
	ListResource
	EndDeviceControl []EndDeviceControl `xml:"EndDeviceControl,omitempty"`
}
