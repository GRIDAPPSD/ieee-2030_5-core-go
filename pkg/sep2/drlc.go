package sep2

import "encoding/xml"

// DemandResponseProgram is a demand response program.
// Spec reference: section 10.3 (stub: handlers deferred)
//
// Field order is the sep.xsd sequence: IdentifiedObject, then the two links
// and thresholds, then primacy.
type DemandResponseProgram struct {
	XMLName xml.Name `xml:"urn:ieee:std:2030.5:ns DemandResponseProgram"`
	Resource
	IdentifiedObject
	ActiveEndDeviceControlListLink           *ListLink    `xml:"ActiveEndDeviceControlListLink,omitempty"`
	AvailabilityUpdatePercentChangeThreshold *PerCent     `xml:"availabilityUpdatePercentChangeThreshold,omitempty"`
	AvailabilityUpdatePowerChangeThreshold   *ActivePower `xml:"availabilityUpdatePowerChangeThreshold,omitempty"`
	EndDeviceControlListLink                 *ListLink    `xml:"EndDeviceControlListLink,omitempty"`
	Primacy                                  uint8        `xml:"primacy"`
}

// Copy returns an independent copy.
func (d DemandResponseProgram) Copy() DemandResponseProgram {
	c := d
	c.IdentifiedObject = d.IdentifiedObject.Copy()
	c.ActiveEndDeviceControlListLink = clonePtr(d.ActiveEndDeviceControlListLink)
	c.AvailabilityUpdatePercentChangeThreshold = clonePtr(d.AvailabilityUpdatePercentChangeThreshold)
	c.AvailabilityUpdatePowerChangeThreshold = clonePtr(d.AvailabilityUpdatePowerChangeThreshold)
	c.EndDeviceControlListLink = clonePtr(d.EndDeviceControlListLink)
	return c
}

// DemandResponseProgramList is a list of DemandResponseProgram resources.
type DemandResponseProgramList struct {
	XMLName xml.Name `xml:"urn:ieee:std:2030.5:ns DemandResponseProgramList"`
	ListResource
	DemandResponseProgram []DemandResponseProgram `xml:"DemandResponseProgram,omitempty"`
}

// EndDeviceControl is a demand response control event.
type EndDeviceControl struct {
	XMLName xml.Name `xml:"urn:ieee:std:2030.5:ns EndDeviceControl"`
	RandomizableEvent
	DeviceCategory *DeviceCategoryType `xml:"deviceCategory,omitempty"`
}

// Copy returns an independent copy.
func (e EndDeviceControl) Copy() EndDeviceControl {
	c := e
	c.Event = e.Event.Copy()
	c.DeviceCategory = clonePtr(e.DeviceCategory)
	c.RandomizeDuration = clonePtr(e.RandomizeDuration)
	c.RandomizeStart = clonePtr(e.RandomizeStart)
	return c
}

// EndDeviceControlList is a list of EndDeviceControl resources.
type EndDeviceControlList struct {
	XMLName xml.Name `xml:"urn:ieee:std:2030.5:ns EndDeviceControlList"`
	ListResource
	EndDeviceControl []EndDeviceControl `xml:"EndDeviceControl,omitempty"`
}
