package xsdgate

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// xsdIntRange describes the value space of an XSD built-in integer type.
type xsdIntRange struct {
	signed bool
	min    int64
	max    int64
	umax   uint64
}

var xsdIntRanges = map[string]xsdIntRange{
	"byte":          {signed: true, min: math.MinInt8, max: math.MaxInt8},
	"short":         {signed: true, min: math.MinInt16, max: math.MaxInt16},
	"int":           {signed: true, min: math.MinInt32, max: math.MaxInt32},
	"long":          {signed: true, min: math.MinInt64, max: math.MaxInt64},
	"unsignedByte":  {umax: math.MaxUint8},
	"unsignedShort": {umax: math.MaxUint16},
	"unsignedInt":   {umax: math.MaxUint32},
	"unsignedLong":  {umax: math.MaxUint64},
}

// isXSDBuiltin reports whether name is an XSD built-in this gate understands.
// Built-ins outside this set cause the value check to be skipped rather than
// to fail, since an unrecognised base is a gap in the gate, not a defect in
// the document.
func isXSDBuiltin(name string) bool {
	if _, ok := xsdIntRanges[name]; ok {
		return true
	}
	switch name {
	case "hexBinary", "string", "boolean", "anyURI", "decimal", "float", "double":
		return true
	}
	return false
}

// checkLexical validates value against the XSD built-in that typeName bottoms
// out at. It returns a human-readable problem description, or "" when the
// value is acceptable (or when the type is outside the checked set).
//
// Note the deliberate asymmetry: hexBinary is checked strictly because
// decimal-where-hex is a silent, consequential defect this module has
// actually shipped, while string and anyURI are effectively unconstrained
// and are not checked at all.
func (s *Schema) checkLexical(typeName, value string) string {
	builtin, maxLen := s.resolveBuiltin(typeName)
	if builtin == "" {
		return ""
	}

	switch builtin {
	case "hexBinary":
		return checkHexBinary(typeName, value, maxLen)

	case "boolean":
		switch value {
		case "true", "false", "0", "1":
			return ""
		}
		return fmt.Sprintf("type %s is xs:boolean but value %q is not one of true/false/0/1", typeName, value)

	case "string", "anyURI", "decimal", "float", "double":
		// Not meaningfully constrained by this gate.
		return ""
	}

	r, ok := xsdIntRanges[builtin]
	if !ok {
		return ""
	}
	if strings.TrimSpace(value) != value || value == "" {
		return fmt.Sprintf("type %s is xs:%s but value %q has surrounding whitespace or is empty", typeName, builtin, value)
	}
	if r.signed {
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Sprintf("type %s is xs:%s but value %q is not a valid integer", typeName, builtin, value)
		}
		if n < r.min || n > r.max {
			return fmt.Sprintf("type %s is xs:%s but value %d is outside [%d, %d]", typeName, builtin, n, r.min, r.max)
		}
		return ""
	}
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return fmt.Sprintf("type %s is xs:%s but value %q is not a valid unsigned integer", typeName, builtin, value)
	}
	if n > r.umax {
		return fmt.Sprintf("type %s is xs:%s but value %d exceeds max %d", typeName, builtin, n, r.umax)
	}
	return ""
}

// checkHexBinary validates the xs:hexBinary lexical space: an even-length run
// of hex digits, whose octet count respects the maxLength facet.
//
// IMPORTANT LIMITATION. This catches a decimal value emitted where hex is
// required only when the decimal is distinguishable from hex: an odd digit
// count (every single-digit value, so 0 through 9, and 100 through 999, and
// so on) or a digit sequence exceeding maxLength. It CANNOT catch an
// even-length all-digit decimal, because "16" is a syntactically valid
// hexBinary that simply denotes 0x16 rather than decimal 16. No lexical check
// can distinguish those two, since they differ only in intent. Fields whose
// value space makes that collision plausible need a value-level assertion in
// the calling test, not just this gate.
func checkHexBinary(typeName, value string, maxLen int) string {
	if value == "" {
		// The empty string is a valid zero-length hexBinary.
		return ""
	}
	if len(value)%2 != 0 {
		return fmt.Sprintf("type %s is xs:hexBinary but value %q has an odd digit count (%d); "+
			"hexBinary is a whole number of octets, so this is very likely a decimal value emitted where hex is required",
			typeName, value, len(value))
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
		if !isHex {
			return fmt.Sprintf("type %s is xs:hexBinary but value %q contains non-hex character %q at offset %d",
				typeName, value, string(c), i)
		}
	}
	if maxLen >= 0 {
		if octets := len(value) / 2; octets > maxLen {
			return fmt.Sprintf("type %s is xs:hexBinary with maxLength %d octet(s) but value %q is %d octet(s)",
				typeName, maxLen, value, octets)
		}
	}
	return ""
}
