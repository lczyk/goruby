package object

import (
	"math/big"
	"strconv"
)

// Integer represents a ruby Integer value. Matches MRI 2.4+'s unified
// Integer class: small values live inline in Value (fast int64 path),
// while values outside int64 range spill into Bn -- a heap-allocated
// big.Int. Callers check IsBig() before reading Value directly.
//
// Pointer-free in the inline case (Bn nil); noscan-eligible until the
// big.Int slot is populated.
type Integer struct {
	Value int64
	// Bn is non-nil exactly when the integer doesn't fit in int64
	// (or arithmetic promoted it past int64 range). When Bn is set,
	// Value is meaningless and callers must use Bn or ToBig().
	Bn *big.Int
}

// IsBig reports whether this Integer is in bignum (spilled) form.
func (i *Integer) IsBig() bool { return i.Bn != nil }

// ToBig returns a *big.Int with the same value, whether the Integer
// is inline or already spilled. Allocates only in the inline case.
func (i *Integer) ToBig() *big.Int {
	if i.Bn != nil {
		return i.Bn
	}
	return big.NewInt(i.Value)
}

// Int64 returns the inline value and ok=true when the Integer fits in
// int64; ok=false when the value lives in Bn and would lose precision.
// Callers that must use the int64 directly (array indexing, etc.)
// should check ok and raise if not.
func (i *Integer) Int64() (int64, bool) {
	if i.Bn == nil {
		return i.Value, true
	}
	if i.Bn.IsInt64() {
		return i.Bn.Int64(), true
	}
	return 0, false
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

// NewBigInteger returns an Integer carrying b. If b fits in int64 the
// result is the equivalent inline Integer (so arithmetic naturally
// demotes back to the fast path); otherwise the bn is held as-is.
// Callers that pass a big.Int the cache range covers still benefit
// from the smallInts table.
func NewBigInteger(b *big.Int) *Integer {
	if b.IsInt64() {
		return NewInteger(b.Int64())
	}
	return &Integer{Bn: new(big.Int).Set(b)}
}

func (i *Integer) Inspect() string {
	if i.Bn != nil {
		return i.Bn.Text(10)
	}
	return strconv.FormatInt(i.Value, 10)
}
func (i *Integer) Type() Type       { return INTEGER_OBJ }
func (i *Integer) Class() RubyClass { return IntegerClass }
