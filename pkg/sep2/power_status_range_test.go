package sep2_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// TestRealEnergyRangeGuard asserts RealEnergy.Value marshals successfully
// within XSD UInt48 range (0..2^48-1) and errors, rather than silently
// serializing an illegal wire value, when out of range.
func TestRealEnergyRangeGuard(t *testing.T) {
	const maxUint48 = 1<<48 - 1

	t.Run("in range max", func(t *testing.T) {
		e := sep2.RealEnergy{Multiplier: 0, Value: maxUint48}
		data, err := xml.Marshal(&e)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !strings.Contains(string(data), "<value>281474976710655</value>") {
			t.Errorf("missing expected value element: %s", data)
		}
	})

	t.Run("in range zero", func(t *testing.T) {
		e := sep2.RealEnergy{Multiplier: 0, Value: 0}
		data, err := xml.Marshal(&e)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !strings.Contains(string(data), "<value>0</value>") {
			t.Errorf("missing expected value element: %s", data)
		}
	})

	t.Run("negative is out of range", func(t *testing.T) {
		e := sep2.RealEnergy{Multiplier: 0, Value: -1}
		_, err := xml.Marshal(&e)
		if err == nil {
			t.Fatal("expected error for negative RealEnergy.Value, got nil")
		}
		if !strings.Contains(err.Error(), "out of UInt48 range") {
			t.Errorf("error %q does not mention UInt48 range", err.Error())
		}
	})

	t.Run("above max is out of range", func(t *testing.T) {
		e := sep2.RealEnergy{Multiplier: 0, Value: maxUint48 + 1}
		_, err := xml.Marshal(&e)
		if err == nil {
			t.Fatal("expected error for RealEnergy.Value above UInt48 max, got nil")
		}
		if !strings.Contains(err.Error(), "out of UInt48 range") {
			t.Errorf("error %q does not mention UInt48 range", err.Error())
		}
	})
}

// TestSignedRealEnergyRangeGuard asserts SignedRealEnergy.Value marshals
// successfully within XSD Int48 range (+-2^47), preserves sign, and errors
// when out of range.
func TestSignedRealEnergyRangeGuard(t *testing.T) {
	const maxInt48 = 1<<47 - 1
	const minInt48 = -(1 << 47)

	t.Run("in range positive max", func(t *testing.T) {
		e := sep2.SignedRealEnergy{Multiplier: 0, Value: maxInt48}
		data, err := xml.Marshal(&e)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !strings.Contains(string(data), "<value>140737488355327</value>") {
			t.Errorf("missing expected value element: %s", data)
		}
	})

	t.Run("in range negative min preserves sign", func(t *testing.T) {
		e := sep2.SignedRealEnergy{Multiplier: 0, Value: minInt48}
		data, err := xml.Marshal(&e)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !strings.Contains(string(data), "<value>-140737488355328</value>") {
			t.Errorf("missing expected negative value element: %s", data)
		}
	})

	t.Run("below min is out of range", func(t *testing.T) {
		e := sep2.SignedRealEnergy{Multiplier: 0, Value: minInt48 - 1}
		_, err := xml.Marshal(&e)
		if err == nil {
			t.Fatal("expected error for SignedRealEnergy.Value below Int48 min, got nil")
		}
		if !strings.Contains(err.Error(), "out of Int48 range") {
			t.Errorf("error %q does not mention Int48 range", err.Error())
		}
	})

	t.Run("above max is out of range", func(t *testing.T) {
		e := sep2.SignedRealEnergy{Multiplier: 0, Value: maxInt48 + 1}
		_, err := xml.Marshal(&e)
		if err == nil {
			t.Fatal("expected error for SignedRealEnergy.Value above Int48 max, got nil")
		}
		if !strings.Contains(err.Error(), "out of Int48 range") {
			t.Errorf("error %q does not mention Int48 range", err.Error())
		}
	})
}
