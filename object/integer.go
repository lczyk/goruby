package object

import "strconv"

// Integer represents a ruby Integer value.
// Pointer-free; noscan-eligible.
type Integer struct {
	Value int64
}

// Small-integer cache range. The cache pre-allocates Integer values for
// every n in [smallIntMin, smallIntMax] inclusive so the very-common path
// (loop counters, small literals, comparison results) never allocates.
// Tunable; benchmarks should decide the upper bound once eval lands.
const (
	smallIntMin = -128
	smallIntMax = 1152
)

// smallInts holds the pre-allocated cache. Backing array is one
// noscan-eligible chunk, reachable for the program's lifetime.
var smallInts [smallIntMax - smallIntMin + 1]Integer

func init() {
	for i := range smallInts {
		smallInts[i] = Integer{Value: int64(i + smallIntMin)}
	}
}

// NewInteger returns a pointer to an Integer with value v. Values in the
// small-integer cache range return a pointer into the shared cache; outside
// that range allocates fresh.
func NewInteger(v int64) *Integer {
	if v >= smallIntMin && v <= smallIntMax {
		return &smallInts[v-smallIntMin]
	}
	return &Integer{Value: v}
}

func (i *Integer) Inspect() string  { return strconv.FormatInt(i.Value, 10) }
func (i *Integer) Type() Type       { return INTEGER_OBJ }
func (i *Integer) Class() RubyClass { return nil }
