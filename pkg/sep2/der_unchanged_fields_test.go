package sep2_test

import (
	"encoding/xml"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// TestDERControlBaseUnchangedFieldsWireBytes pins the exact wire bytes of
// every DERControlBase field outside the eleven retyped for #153, so the
// retyping cannot move or reshape a neighbour.
func TestDERControlBaseUnchangedFieldsWireBytes(t *testing.T) {
	connect, energize := true, false
	absorb := sep2.FixedPowerFactor{Displacement: 950, Excitation: true, Multiplier: -3}
	inject := sep2.FixedPowerFactor{Displacement: 900, Excitation: false, Multiplier: -3}
	fixedW := sep2.SignedPerCent(-2500)
	maxLim := sep2.PerCent(7500)
	targetVar := sep2.ReactivePower{Multiplier: 1, Value: -120}
	targetW := sep2.ActivePower{Multiplier: 2, Value: 45}
	ramp := uint16(300)
	base := sep2.DERControlBase{
		OpModConnect:        &connect,
		OpModEnergize:       &energize,
		OpModFixedPFAbsorbW: &absorb,
		OpModFixedPFInjectW: &inject,
		OpModFixedW:         &fixedW,
		OpModMaxLimW:        &maxLim,
		OpModTargetVar:      &targetVar,
		OpModTargetW:        &targetW,
		RampTms:             &ramp,
	}

	got, err := xml.Marshal(&base)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `<DERControlBase><opModConnect>true</opModConnect><opModEnergize>false</opModEnergize>` +
		`<opModFixedPFAbsorbW><displacement>950</displacement><excitation>true</excitation><multiplier>-3</multiplier></opModFixedPFAbsorbW><opModFixedPFInjectW><displacement>900</displacement><excitation>false</excitation><multiplier>-3</multiplier></opModFixedPFInjectW>` +
		`<opModFixedW>-2500</opModFixedW><opModMaxLimW>7500</opModMaxLimW><opModTargetVar><multiplier>1</multiplier><value>-120</value></opModTargetVar><opModTargetW><multiplier>2</multiplier><value>45</value></opModTargetW><rampTms>300</rampTms></DERControlBase>`
	if string(got) != want {
		t.Errorf("wire bytes changed\n got: %s\nwant: %s", got, want)
	}
}
