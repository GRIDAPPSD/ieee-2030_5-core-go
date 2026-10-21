package sep2_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

// TestEventStatusConstantValues pins every currentStatus constant to the
// numeric value sep.xsd documents for it, and checks the serialized element.
func TestEventStatusConstantValues(t *testing.T) {
	tests := []struct {
		name string
		got  uint8
		want uint8
	}{
		{"Scheduled", sep2.EventStatusScheduled, 0},
		{"Active", sep2.EventStatusActive, 1},
		{"Cancelled", sep2.EventStatusCancelled, 2},
		{"CancelledWithRandomization", sep2.EventStatusCancelledWithRandomization, 3},
		{"Superseded", sep2.EventStatusSuperseded, 4},
		{"Complete", sep2.EventStatusComplete, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("constant = %d, want %d", tt.got, tt.want)
			}
			out, err := xml.Marshal(sep2.EventStatus{CurrentStatus: tt.got, DateTime: 1700000000})
			if err != nil {
				t.Fatal(err)
			}
			wantElem := "<currentStatus>" + string(rune('0'+tt.want)) + "</currentStatus>"
			if !strings.Contains(string(out), wantElem) {
				t.Fatalf("wire = %s, want it to contain %s", out, wantElem)
			}
		})
	}
}
