package object

import (
	"math"
	"strconv"
)

// Float represents a ruby Float value.
// Pointer-free; noscan-eligible.
type Float struct {
	Value float64
}

// NewFloat returns a pointer to a Float with value v.
func NewFloat(v float64) *Float { return &Float{Value: v} }

func (f *Float) Inspect() string {
	// MRI's special cases.
	if math.IsNaN(f.Value) {
		return "NaN"
	}
	if math.IsInf(f.Value, 1) {
		return "Infinity"
	}
	if math.IsInf(f.Value, -1) {
		return "-Infinity"
	}
	// MRI prints integral floats as "1.0", not "1". Match that.
	s := strconv.FormatFloat(f.Value, 'g', -1, 64)
	for i := 0; i < len(s); i++ {
		if s[i] == '.' || s[i] == 'e' {
			return s
		}
	}
	return s + ".0"
}

func (f *Float) Type() Type       { return FLOAT_OBJ }
func (f *Float) Class() RubyClass { return nil }
