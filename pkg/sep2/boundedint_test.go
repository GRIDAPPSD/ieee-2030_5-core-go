package sep2

import (
	"encoding/json"
	"encoding/xml"
	"strconv"
	"strings"
	"testing"
)

// boundedCase drives one bounded integer type through both encodings.
type boundedCase struct {
	name    string
	lo, hi  int64
	decodeX func(text string) (int64, error)
	decodeJ func(text string) (int64, error)
	encodeX func(v int64) (string, error)
	encodeJ func(v int64) (string, error)
}

func boundedCases() []boundedCase {
	return []boundedCase{
		{
			name: "PerCent", lo: 0, hi: 10000,
			decodeX: func(s string) (int64, error) {
				var v PerCent
				err := xml.Unmarshal([]byte("<v>"+s+"</v>"), &v)
				return int64(v), err
			},
			decodeJ: func(s string) (int64, error) {
				var v PerCent
				err := json.Unmarshal([]byte(s), &v)
				return int64(v), err
			},
			encodeX: func(v int64) (string, error) {
				b, err := xml.Marshal(PerCent(v))
				return string(b), err
			},
			encodeJ: func(v int64) (string, error) {
				b, err := json.Marshal(PerCent(v))
				return string(b), err
			},
		},
		{
			name: "SignedPerCent", lo: -10000, hi: 10000,
			decodeX: func(s string) (int64, error) {
				var v SignedPerCent
				err := xml.Unmarshal([]byte("<v>"+s+"</v>"), &v)
				return int64(v), err
			},
			decodeJ: func(s string) (int64, error) {
				var v SignedPerCent
				err := json.Unmarshal([]byte(s), &v)
				return int64(v), err
			},
			encodeX: func(v int64) (string, error) {
				b, err := xml.Marshal(SignedPerCent(v))
				return string(b), err
			},
			encodeJ: func(v int64) (string, error) {
				b, err := json.Marshal(SignedPerCent(v))
				return string(b), err
			},
		},
		{
			name: "OneHourRange", lo: -3600, hi: 3600,
			decodeX: func(s string) (int64, error) {
				var v OneHourRange
				err := xml.Unmarshal([]byte("<v>"+s+"</v>"), &v)
				return int64(v), err
			},
			decodeJ: func(s string) (int64, error) {
				var v OneHourRange
				err := json.Unmarshal([]byte(s), &v)
				return int64(v), err
			},
			encodeX: func(v int64) (string, error) {
				b, err := xml.Marshal(OneHourRange(v))
				return string(b), err
			},
			encodeJ: func(v int64) (string, error) {
				b, err := json.Marshal(OneHourRange(v))
				return string(b), err
			},
		},
	}
}

func rangeText(name string, v, lo, hi int64) string {
	return "sep2: " + name + " " + strconv.FormatInt(v, 10) +
		" out of range [" + strconv.FormatInt(lo, 10) + ", " + strconv.FormatInt(hi, 10) + "]"
}

// TestBoundedIntDecodeBoundaries pins the lowest and highest accepted values,
// one below and one above, and the exact refusal text, for each type in XML
// and JSON.
func TestBoundedIntDecodeBoundaries(t *testing.T) {
	for _, c := range boundedCases() {
		for _, enc := range []string{"xml", "json"} {
			dec := c.decodeX
			if enc == "json" {
				dec = c.decodeJ
			}
			for _, ok := range []int64{c.lo, c.hi} {
				got, err := dec(strconv.FormatInt(ok, 10))
				if err != nil || got != ok {
					t.Errorf("%s %s accept %d: got %d, %v", c.name, enc, ok, got, err)
				}
			}
			for _, bad := range []int64{c.lo - 1, c.hi + 1} {
				got, err := dec(strconv.FormatInt(bad, 10))
				if err == nil {
					t.Errorf("%s %s %d: want refusal, got %d", c.name, enc, bad, got)
					continue
				}
				if want := rangeText(c.name, bad, c.lo, c.hi); err.Error() != want {
					t.Errorf("%s %s %d: error %q, want %q", c.name, enc, bad, err.Error(), want)
				}
			}
		}
	}
}

// TestBoundedIntEncodeBoundaries pins the wire bytes for valid extremes and
// the refusal text for one below and one above, in both encodings.
func TestBoundedIntEncodeBoundaries(t *testing.T) {
	for _, c := range boundedCases() {
		for _, ok := range []int64{c.lo, c.hi, 0} {
			s := strconv.FormatInt(ok, 10)
			if got, err := c.encodeJ(ok); err != nil || got != s {
				t.Errorf("%s json encode %d: %q, %v", c.name, ok, got, err)
			}
			wantX := "<" + c.name + ">" + s + "</" + c.name + ">"
			if got, err := c.encodeX(ok); err != nil || got != wantX {
				t.Errorf("%s xml encode %d: %q, want %q, %v", c.name, ok, got, wantX, err)
			}
		}
		for _, bad := range []int64{c.lo - 1, c.hi + 1} {
			if c.lo == 0 && bad < 0 {
				continue // PerCent is unsigned; -1 is not representable
			}
			want := rangeText(c.name, bad, c.lo, c.hi)
			if _, err := c.encodeJ(bad); err == nil || !strings.HasSuffix(err.Error(), want) {
				t.Errorf("%s json encode %d: %v, want %q", c.name, bad, err, want)
			}
			if _, err := c.encodeX(bad); err == nil || err.Error() != want {
				t.Errorf("%s xml encode %d: %v, want %q", c.name, bad, err, want)
			}
		}
	}
}

// TestBoundedIntScalarRefusals pins the empty and non-numeric XML refusals.
func TestBoundedIntScalarRefusals(t *testing.T) {
	for _, c := range boundedCases() {
		if _, err := c.decodeX("  "); err == nil ||
			err.Error() != "sep2: "+c.name+": empty or non-scalar element" {
			t.Errorf("%s empty: %v", c.name, err)
		}
		if _, err := c.decodeX("abc"); err == nil {
			t.Errorf("%s non-numeric: want error", c.name)
		}
		if got, err := c.decodeX(" 5 "); err != nil || got != 5 {
			t.Errorf("%s padded: %d, %v", c.name, got, err)
		}
	}
}
