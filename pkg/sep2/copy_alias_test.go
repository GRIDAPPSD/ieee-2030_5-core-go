package sep2

import (
	"encoding/xml"
	"reflect"
	"testing"
)

// fillAll sets every settable field of v to a non-zero value, allocating
// every pointer and slice, so a Copy that misses any of them is visible.
func fillAll(v reflect.Value, depth int) {
	if depth > 8 {
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		fillAll(v.Elem(), depth+1)
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(xml.Name{}) {
			return
		}
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).CanSet() {
				fillAll(v.Field(i), depth+1)
			}
		}
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 1, 1))
		fillAll(v.Index(0), depth+1)
	case reflect.String:
		v.SetString("x")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(1)
	}
}

// addrs records the address of every pointer target and slice backing array
// reachable from v, with the path that led there.
func addrs(v reflect.Value, path string, out map[uintptr]string, depth int) {
	if depth > 8 {
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		out[v.Pointer()] = path
		addrs(v.Elem(), path+"*", out, depth+1)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			addrs(v.Field(i), path+"."+v.Type().Field(i).Name, out, depth+1)
		}
	case reflect.Slice:
		if v.Len() == 0 {
			return
		}
		out[v.Pointer()] = path
		for i := 0; i < v.Len(); i++ {
			addrs(v.Index(i), path+"[]", out, depth+1)
		}
	}
}

// assertCopyIndependent fills a T, copies it, and fails when the copy shares
// any pointer or slice with the original, or marshals to different bytes.
// A type with no pointer fields passes flat; if a pointer is later added the
// alias check runs on it regardless.
func assertCopyIndependent[T interface{ Copy() T }](t *testing.T, name string, flat ...bool) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		var orig T
		fillAll(reflect.ValueOf(&orig).Elem(), 0)
		cp := orig.Copy()

		before := map[uintptr]string{}
		after := map[uintptr]string{}
		addrs(reflect.ValueOf(orig), "", before, 0)
		addrs(reflect.ValueOf(cp), "", after, 0)
		if len(before) == 0 && len(flat) == 0 {
			t.Fatalf("fixture for %s holds no pointers; the check cannot fail", name)
		}
		for p, path := range after {
			if src, ok := before[p]; ok {
				t.Errorf("copy aliases the original at %s (original path %s)", path, src)
			}
		}

		want, werr := xml.Marshal(orig)
		got, gerr := xml.Marshal(cp)
		if (werr == nil) != (gerr == nil) || string(want) != string(got) {
			t.Errorf("copy marshals differently:\n orig %s\n copy %s", want, got)
		}
	})
}

func TestCopyIndependent(t *testing.T) {
	assertCopyIndependent[Configuration](t, "Configuration")
	assertCopyIndependent[DERControl](t, "DERControl")
	assertCopyIndependent[DefaultDERControl](t, "DefaultDERControl")
	assertCopyIndependent[DERProgram](t, "DERProgram")
	assertCopyIndependent[DER](t, "DER")
	assertCopyIndependent[DERSettings](t, "DERSettings")
	assertCopyIndependent[DERStatus](t, "DERStatus")
	assertCopyIndependent[DERAvailability](t, "DERAvailability")
	assertCopyIndependent[DemandResponseProgram](t, "DemandResponseProgram")
	assertCopyIndependent[EndDeviceControl](t, "EndDeviceControl")
	assertCopyIndependent[FlowReservationResponse](t, "FlowReservationResponse")
	assertCopyIndependent[TextMessage](t, "TextMessage")
	assertCopyIndependent[MessagingProgram](t, "MessagingProgram")
	assertCopyIndependent[SelfDevice](t, "SelfDevice")
	assertCopyIndependent[EndDevice](t, "EndDevice")
	assertCopyIndependent[Subscription](t, "Subscription")
	assertCopyIndependent[UsagePoint](t, "UsagePoint")
	assertCopyIndependent[MeterReading](t, "MeterReading")
	assertCopyIndependent[Event](t, "Event")
	assertCopyIndependent[SubscribableResource](t, "SubscribableResource")
	assertCopyIndependent[DERControlBase](t, "DERControlBase")
	assertCopyIndependent[DERCapability](t, "DERCapability")
	assertCopyIndependent[DERCurve](t, "DERCurve")
	assertCopyIndependent[DeviceInformation](t, "DeviceInformation")
	assertCopyIndependent[DeviceStatus](t, "DeviceStatus")
	assertCopyIndependent[ReadingType](t, "ReadingType")
	assertCopyIndependent[Reading](t, "Reading")
	assertCopyIndependent[FlowReservationRequest](t, "FlowReservationRequest")
	assertCopyIndependent[FunctionSetAssignments](t, "FunctionSetAssignments")
	assertCopyIndependent[LogEvent](t, "LogEvent")
	assertCopyIndependent[MirrorUsagePoint](t, "MirrorUsagePoint")
	assertCopyIndependent[MirrorMeterReading](t, "MirrorMeterReading")
	assertCopyIndependent[MirrorReadingSet](t, "MirrorReadingSet")
	assertCopyIndependent[PowerStatus](t, "PowerStatus")
	assertCopyIndependent[Registration](t, "Registration", true)
	assertCopyIndependent[Response](t, "Response")
	assertCopyIndependent[ResponseSet](t, "ResponseSet")
	assertCopyIndependent[DERControlResponse](t, "DERControlResponse")
	assertCopyIndependent[FlowReservationResponseResponse](t, "FlowReservationResponseResponse")
	assertCopyIndependent[PriceResponse](t, "PriceResponse")
	assertCopyIndependent[TextResponse](t, "TextResponse")
	assertCopyIndependent[Notification](t, "Notification", true)
	assertCopyIndependent[EventStatus](t, "EventStatus")
	assertCopyIndependent[PowerConfiguration](t, "PowerConfiguration")
}

// aliasingProbe has a pointer field its Copy forgets, standing in for a
// field added to a real type without updating Copy.
type aliasingProbe struct {
	A *uint8
	B *uint8
}

func (p aliasingProbe) Copy() aliasingProbe {
	c := p
	if p.A != nil {
		v := *p.A
		c.A = &v
	}
	return c
}

func TestCopyIndependentCatchesMissedField(t *testing.T) {
	var orig aliasingProbe
	fillAll(reflect.ValueOf(&orig).Elem(), 0)
	cp := orig.Copy()
	before := map[uintptr]string{}
	after := map[uintptr]string{}
	addrs(reflect.ValueOf(orig), "", before, 0)
	addrs(reflect.ValueOf(cp), "", after, 0)
	shared := 0
	for p := range after {
		if _, ok := before[p]; ok {
			shared++
		}
	}
	if shared != 1 {
		t.Fatalf("control: want exactly 1 shared pointer (field B), got %d", shared)
	}
}
