package evaluator

import (
	"math/rand"
	"strings"
	"time"

	"github.com/lczyk/goruby/object"
)

// rng is the evaluator's private PRNG. Backs Kernel#rand, Kernel#srand,
// Array#sample. A package-private *rand.Rand instance keeps seeding
// deterministic without interfering with Go's default source -- and
// avoids the deprecated rand.Seed on the global source.
var rng = rand.New(rand.NewSource(1))

// randomIntn returns a non-negative pseudo-random int in [0, n). Used
// by Array#sample.
func randomIntn(n int) int {
	if n <= 0 {
		return 0
	}
	return rng.Intn(n)
}

// randFloat returns a pseudo-random float in [0, 1). Hook for
// Kernel#rand's no-arg form.
func randFloat() float64 { return rng.Float64() }

// randSeed reseeds the evaluator's PRNG. Exposed for Kernel#srand.
// seed=0 resets to MRI's "use time" default; we approximate with
// time.Now().UnixNano() so reseeding is non-deterministic but not
// identical to the prior state.
func randSeed(seed int64) {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	rng = rand.New(rand.NewSource(seed))
}

func callArrayMethod(env *object.Environment, r *object.Array, name string, args []object.RubyObject) (object.RubyObject, error) {
	switch name {
	case "slice", "[]":
		// MRI: Array#slice is identical to Array#[]; both accept
		// (index), (start, length), or (range). Delegate to the shared
		// arrayIndex used by subscript notation.
		return arrayIndex(r, args)
	case "clear":
		r.Elements = r.Elements[:0]
		return r, nil
	case "fill":
		// MRI Array#fill shapes covered:
		//   arr.fill(value)                       -> fill the whole array
		//   arr.fill(value, start)                -> from start to end (extends)
		//   arr.fill(value, start, length)        -> [start, start+length)
		//   arr.fill(value, range)                -> indices in the range
		// Block forms (`arr.fill { |i| ... }`) aren't covered here -- they
		// route through the block-aware dispatcher when added.
		if len(args) == 0 {
			return nil, errorf("evaluator: Array#fill: at least one arg required")
		}
		val := args[0]
		start, length := 0, len(r.Elements)
		switch len(args) {
		case 1:
			// fill all
		case 2:
			if rng, ok := args[1].(*object.Range); ok {
				// Compute bounds without rangeBounds' "clamp hi to n" --
				// fill is allowed to extend past the current end and the
				// loop below grows the slice.
				b, bOK := rng.Begin.(*object.Integer)
				e, eOK := rng.End.(*object.Integer)
				if !bOK || !eOK {
					return nil, errorf("evaluator: Array#fill: Range endpoints must be Integer")
				}
				lo := int(b.Value)
				hi := int(e.Value)
				if lo < 0 {
					lo += len(r.Elements)
				}
				if hi < 0 {
					hi += len(r.Elements)
				}
				if !rng.Exclusive {
					hi++
				}
				if lo < 0 {
					lo = 0
				}
				if hi < lo {
					hi = lo
				}
				start = lo
				length = hi - lo
				break
			}
			n, ok := args[1].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Array#fill: start must be Integer or Range, got %T", args[1])
			}
			start = int(n.Value)
			if start < 0 {
				start += len(r.Elements)
			}
			length = len(r.Elements) - start
			if length < 0 {
				length = 0
			}
		case 3:
			s, ok1 := args[1].(*object.Integer)
			l, ok2 := args[2].(*object.Integer)
			if !ok1 || !ok2 {
				return nil, errorf("evaluator: Array#fill: start and length must be Integer")
			}
			start = int(s.Value)
			if start < 0 {
				start += len(r.Elements)
			}
			length = int(l.Value)
			if length < 0 {
				length = 0
			}
		default:
			return nil, errorf("evaluator: Array#fill: too many args (%d)", len(args))
		}
		if start < 0 {
			start = 0
		}
		end := start + length
		// Grow the slice if the fill range extends past the current size,
		// padding the gap (between old size and start) with nil to match
		// MRI's behaviour.
		for len(r.Elements) < end {
			r.Elements = append(r.Elements, object.NIL)
		}
		for i := start; i < end; i++ {
			r.Elements[i] = val
		}
		return r, nil
	case "rotate", "rotate!":
		// Array#rotate(n=1): rotate left by n (n negative -> rotate right).
		// rotate!  mutates in place; rotate returns a new array.
		n := int64(1)
		if len(args) >= 1 {
			ni, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Array#%s: count must be Integer, got %T", name, args[0])
			}
			n = ni.Value
		}
		size := int64(len(r.Elements))
		if size == 0 {
			if name == "rotate!" {
				return r, nil
			}
			return object.NewArray(), nil
		}
		n = ((n % size) + size) % size
		rotated := make([]object.RubyObject, 0, size)
		rotated = append(rotated, r.Elements[n:]...)
		rotated = append(rotated, r.Elements[:n]...)
		if name == "rotate!" {
			r.Elements = rotated
			return r, nil
		}
		return object.NewArray(rotated...), nil
	case "transpose":
		// Array#transpose: receiver must be an array of equal-length
		// arrays. Returns the transposed array.
		if len(r.Elements) == 0 {
			return object.NewArray(), nil
		}
		rows := make([]*object.Array, len(r.Elements))
		width := -1
		for i, e := range r.Elements {
			row, ok := e.(*object.Array)
			if !ok {
				return nil, errorf("evaluator: Array#transpose: element %d not Array (%T)", i, e)
			}
			if width == -1 {
				width = len(row.Elements)
			} else if width != len(row.Elements) {
				return nil, errorf("evaluator: IndexError: element size differs (%d should be %d)", len(row.Elements), width)
			}
			rows[i] = row
		}
		out := make([]object.RubyObject, width)
		for c := 0; c < width; c++ {
			col := make([]object.RubyObject, len(rows))
			for ri, row := range rows {
				col[ri] = row.Elements[c]
			}
			out[c] = object.NewArray(col...)
		}
		return object.NewArray(out...), nil
	case "fetch":
		if len(args) < 1 || len(args) > 2 {
			return nil, errorf("evaluator: Array#fetch expects 1..2 args, got %d", len(args))
		}
		idx, ok := args[0].(*object.Integer)
		if !ok {
			return nil, errorf("evaluator: Array#fetch index must be Integer, got %T", args[0])
		}
		i := int(idx.Value)
		if i < 0 {
			i += len(r.Elements)
		}
		if i >= 0 && i < len(r.Elements) {
			return r.Elements[i], nil
		}
		if len(args) == 2 {
			return args[1], nil
		}
		return raiseBuiltin(env, "IndexError", "index out of array bounds")
	case "sample":
		// Array#sample: pick a random element. Without args returns
		// nil on empty. We don't track an Rng seed; use math/rand's
		// default source. Stub is OK for fixture parity when programs
		// either don't hit sample, or accept any element (e.g.
		// labyrinth's neighbors.sample when two opposing directions
		// are both legal).
		if len(r.Elements) == 0 {
			return object.NIL, nil
		}
		return r.Elements[randomIntn(len(r.Elements))], nil
	case "length", "size", "count":
		if name == "count" && len(args) == 1 {
			n := 0
			for _, e := range r.Elements {
				if rubyEqual(e, args[0]) {
					n++
				}
			}
			return object.NewInteger(int64(n)), nil
		}
		return object.NewInteger(int64(len(r.Elements))), nil
	case "first":
		if len(args) == 1 {
			n, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Array#first(n) needs Integer")
			}
			cnt := int(n.Value)
			if cnt < 0 {
				return nil, errorf("evaluator: ArgumentError: negative array size")
			}
			if cnt > len(r.Elements) {
				cnt = len(r.Elements)
			}
			out := make([]object.RubyObject, cnt)
			copy(out, r.Elements[:cnt])
			return object.NewArray(out...), nil
		}
		if len(r.Elements) == 0 {
			return object.NIL, nil
		}
		return r.Elements[0], nil
	case "last":
		if len(args) == 1 {
			n, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Array#last(n) needs Integer")
			}
			cnt := int(n.Value)
			if cnt < 0 {
				return nil, errorf("evaluator: ArgumentError: negative array size")
			}
			if cnt > len(r.Elements) {
				cnt = len(r.Elements)
			}
			out := make([]object.RubyObject, cnt)
			copy(out, r.Elements[len(r.Elements)-cnt:])
			return object.NewArray(out...), nil
		}
		if len(r.Elements) == 0 {
			return object.NIL, nil
		}
		return r.Elements[len(r.Elements)-1], nil
	case "to_ary":
		// MRI: returns self (Array implements the to_ary contract).
		// Lets `x.respond_to?(:to_ary) ? x.to_ary : [x]` shapes
		// short-circuit to the existing array.
		return r, nil
	case "replace":
		// Array#replace(other) -- mutates self to contain other's
		// elements. Returns self. Used by ARGV.replace([...]) when a
		// test driver wants to re-seed the script args.
		if len(args) != 1 {
			return nil, errorf("evaluator: Array#replace: expected 1 arg, got %d", len(args))
		}
		other, ok := args[0].(*object.Array)
		if !ok {
			return nil, errorf("evaluator: Array#replace: expected Array, got %T", args[0])
		}
		r.Elements = append(r.Elements[:0], other.Elements...)
		return r, nil
	case "values_at":
		// Array#values_at(*indices) -- one slot per index; out-of-
		// range (positive or negative beyond -len) maps to nil. Mirrors
		// MRI w/o the Range-index form (rake never hits it).
		out := make([]object.RubyObject, 0, len(args))
		for _, a := range args {
			n, ok := a.(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Array#values_at: indices must be Integer, got %T", a)
			}
			i := int(n.Value)
			if i < 0 {
				i += len(r.Elements)
			}
			if i < 0 || i >= len(r.Elements) {
				out = append(out, object.NIL)
			} else {
				out = append(out, r.Elements[i])
			}
		}
		return object.NewArray(out...), nil
	case "push", "append":
		r.Elements = append(r.Elements, args...)
		return r, nil
	case "pop":
		if len(r.Elements) == 0 {
			return object.NIL, nil
		}
		v := r.Elements[len(r.Elements)-1]
		r.Elements = r.Elements[:len(r.Elements)-1]
		return v, nil
	case "shift":
		if len(r.Elements) == 0 {
			return object.NIL, nil
		}
		v := r.Elements[0]
		r.Elements = r.Elements[1:]
		return v, nil
	case "unshift", "prepend":
		r.Elements = append(args, r.Elements...)
		return r, nil
	case "delete":
		if len(args) != 1 {
			return nil, errorf("evaluator: Array#delete expects 1 arg, got %d", len(args))
		}
		out := make([]object.RubyObject, 0, len(r.Elements))
		var removed object.RubyObject = object.NIL
		for _, e := range r.Elements {
			if rubyEqual(e, args[0]) {
				removed = e
				continue
			}
			out = append(out, e)
		}
		r.Elements = out
		return removed, nil
	case "delete_at":
		if len(args) != 1 {
			return nil, errorf("evaluator: Array#delete_at expects 1 arg, got %d", len(args))
		}
		idx, ok := args[0].(*object.Integer)
		if !ok {
			return nil, errorf("evaluator: Array#delete_at needs Integer")
		}
		i := int(idx.Value)
		if i < 0 {
			i += len(r.Elements)
		}
		if i < 0 || i >= len(r.Elements) {
			return object.NIL, nil
		}
		v := r.Elements[i]
		r.Elements = append(r.Elements[:i], r.Elements[i+1:]...)
		return v, nil
	case "concat":
		for _, a := range args {
			other, ok := a.(*object.Array)
			if !ok {
				return nil, errorf("evaluator: Array#concat needs Array args")
			}
			r.Elements = append(r.Elements, other.Elements...)
		}
		return r, nil
	case "reverse":
		out := make([]object.RubyObject, len(r.Elements))
		for i, v := range r.Elements {
			out[len(r.Elements)-1-i] = v
		}
		return object.NewArray(out...), nil
	case "shuffle":
		// Fisher-Yates via the eval-shared rand source. Returns a
		// fresh Array; receiver unchanged. With srand-seeded rand
		// the result is deterministic.
		out := make([]object.RubyObject, len(r.Elements))
		copy(out, r.Elements)
		for i := len(out) - 1; i > 0; i-- {
			j := rng.Intn(i + 1)
			out[i], out[j] = out[j], out[i]
		}
		return object.NewArray(out...), nil
	case "reverse!":
		// In-place reverse. Returns the (now-reversed) receiver.
		for i, j := 0, len(r.Elements)-1; i < j; i, j = i+1, j-1 {
			r.Elements[i], r.Elements[j] = r.Elements[j], r.Elements[i]
		}
		return r, nil
	case "sort!":
		if err := sortArray(env, r.Elements); err != nil {
			return nil, err
		}
		return r, nil
	case "uniq!":
		// In-place dedup keeping first occurrence. Returns nil when no
		// duplicates were removed (matches mri).
		seen := make([]object.RubyObject, 0, len(r.Elements))
		for _, e := range r.Elements {
			dup := false
			for _, s := range seen {
				if rubyEqual(s, e) {
					dup = true
					break
				}
			}
			if !dup {
				seen = append(seen, e)
			}
		}
		if len(seen) == len(r.Elements) {
			return object.NIL, nil
		}
		r.Elements = seen
		return r, nil
	case "compact!":
		out := r.Elements[:0]
		for _, e := range r.Elements {
			if _, ok := e.(*object.Nil); !ok {
				out = append(out, e)
			}
		}
		if len(out) == len(r.Elements) {
			return object.NIL, nil
		}
		r.Elements = out
		return r, nil
	case "flatten!":
		// In-place recursive flatten. Optional depth arg matches MRI's
		// Array#flatten!(depth): default -1 means flatten all levels.
		depth := -1
		if len(args) == 1 {
			d, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Array#flatten! depth must be Integer")
			}
			depth = int(d.Value)
		}
		out := flattenArrayDepth(r, depth)
		// MRI returns nil when no change was needed.
		if len(out) == len(r.Elements) {
			same := true
			for i := range out {
				if out[i] != r.Elements[i] {
					same = false
					break
				}
			}
			if same {
				return object.NIL, nil
			}
		}
		r.Elements = out
		return r, nil
	case "sort":
		out := make([]object.RubyObject, len(r.Elements))
		copy(out, r.Elements)
		if err := sortArray(env, out); err != nil {
			return nil, err
		}
		return object.NewArray(out...), nil
	case "index", "find_index":
		// Array#index(val) returns the position of the first matching
		// element, nil if absent. Block form (no arg, with block) is
		// not handled here -- callers that pass a block will hit the
		// block-aware dispatcher.
		if len(args) != 1 {
			return nil, errorf("evaluator: wrong number of arguments to Array#%s (given %d, expected 1)", name, len(args))
		}
		for i, e := range r.Elements {
			if rubyEqualDispatch(env, e, args[0]) {
				return object.NewInteger(int64(i)), nil
			}
		}
		return object.NIL, nil
	case "rindex":
		if len(args) != 1 {
			return nil, errorf("evaluator: wrong number of arguments to Array#rindex (given %d, expected 1)", len(args))
		}
		for i := len(r.Elements) - 1; i >= 0; i-- {
			if rubyEqualDispatch(env, r.Elements[i], args[0]) {
				return object.NewInteger(int64(i)), nil
			}
		}
		return object.NIL, nil
	case "assoc":
		if len(args) != 1 {
			return nil, errorf("evaluator: wrong number of arguments to Array#assoc (given %d, expected 1)", len(args))
		}
		for _, e := range r.Elements {
			arr, ok := e.(*object.Array)
			if !ok || len(arr.Elements) == 0 {
				continue
			}
			if rubyEqualDispatch(env, arr.Elements[0], args[0]) {
				return arr, nil
			}
		}
		return object.NIL, nil
	case "rassoc":
		if len(args) != 1 {
			return nil, errorf("evaluator: wrong number of arguments to Array#rassoc (given %d, expected 1)", len(args))
		}
		for _, e := range r.Elements {
			arr, ok := e.(*object.Array)
			if !ok || len(arr.Elements) < 2 {
				continue
			}
			if rubyEqualDispatch(env, arr.Elements[1], args[0]) {
				return arr, nil
			}
		}
		return object.NIL, nil
	case "lazy":
		return lazyFromArray(r), nil
	case "product":
		// Cartesian product of self with each Array in args.
		others := make([][]object.RubyObject, 0, len(args)+1)
		others = append(others, r.Elements)
		for _, a := range args {
			arr, ok := a.(*object.Array)
			if !ok {
				return nil, errorf("evaluator: Array#product needs Array args, got %T", a)
			}
			others = append(others, arr.Elements)
		}
		out := []object.RubyObject{}
		acc := []object.RubyObject{}
		var rec func(depth int)
		rec = func(depth int) {
			if depth == len(others) {
				tuple := make([]object.RubyObject, len(acc))
				copy(tuple, acc)
				out = append(out, object.NewArray(tuple...))
				return
			}
			for _, e := range others[depth] {
				acc = append(acc, e)
				rec(depth + 1)
				acc = acc[:len(acc)-1]
			}
		}
		rec(0)
		return object.NewArray(out...), nil
	case "combination":
		if len(args) != 1 {
			return nil, errorf("evaluator: Array#combination expects 1 arg, got %d", len(args))
		}
		k, ok := args[0].(*object.Integer)
		if !ok {
			return nil, errorf("evaluator: Array#combination needs Integer")
		}
		return arrayOfArrays(combinations(r.Elements, int(k.Value))), nil
	case "permutation":
		k := int64(len(r.Elements))
		if len(args) == 1 {
			n, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Array#permutation needs Integer")
			}
			k = n.Value
		}
		return arrayOfArrays(permutations(r.Elements, int(k))), nil
	case "include?", "member?":
		if len(args) != 1 {
			return nil, errorf("evaluator: wrong number of arguments to Array#%s (given %d, expected 1)", name, len(args))
		}
		for _, v := range r.Elements {
			if rubyEqualDispatch(env, v, args[0]) {
				return object.TRUE, nil
			}
		}
		return object.FALSE, nil
	case "empty?":
		return object.BooleanOf(len(r.Elements) == 0), nil
	case "any?":
		// any?(pat) -> any { |e| pat === e }; any? alone -> truthy.
		if len(args) == 1 {
			for _, e := range r.Elements {
				if caseEqual(env, args[0], e) {
					return object.TRUE, nil
				}
			}
			return object.FALSE, nil
		}
		for _, e := range r.Elements {
			if truthy(e) {
				return object.TRUE, nil
			}
		}
		return object.FALSE, nil
	case "all?":
		if len(args) == 1 {
			for _, e := range r.Elements {
				if !caseEqual(env, args[0], e) {
					return object.FALSE, nil
				}
			}
			return object.TRUE, nil
		}
		for _, e := range r.Elements {
			if !truthy(e) {
				return object.FALSE, nil
			}
		}
		return object.TRUE, nil
	case "none?":
		if len(args) == 1 {
			for _, e := range r.Elements {
				if caseEqual(env, args[0], e) {
					return object.FALSE, nil
				}
			}
			return object.TRUE, nil
		}
		for _, e := range r.Elements {
			if truthy(e) {
				return object.FALSE, nil
			}
		}
		return object.TRUE, nil
	case "one?":
		n := 0
		for _, e := range r.Elements {
			if truthy(e) {
				n++
				if n > 1 {
					return object.FALSE, nil
				}
			}
		}
		return object.BooleanOf(n == 1), nil
	case "to_h":
		entries := make([]object.HashEntry, 0, len(r.Elements))
		for _, e := range r.Elements {
			pair, ok := e.(*object.Array)
			if !ok || len(pair.Elements) != 2 {
				return nil, errorf("evaluator: TypeError: wrong element type for to_h (expected 2-element Array)")
			}
			entries = append(entries, object.HashEntry{Key: pair.Elements[0], Value: pair.Elements[1]})
		}
		return object.NewHash(entries...), nil
	case "to_a":
		return r, nil
	case "dig":
		var cur object.RubyObject = r
		for _, k := range args {
			switch x := cur.(type) {
			case *object.Array:
				ki, ok := k.(*object.Integer)
				if !ok {
					return nil, errorf("evaluator: Array#dig needs Integer key")
				}
				idx := int(ki.Value)
				if idx < 0 {
					idx += len(x.Elements)
				}
				if idx < 0 || idx >= len(x.Elements) {
					return object.NIL, nil
				}
				cur = x.Elements[idx]
			case *object.Hash:
				var found object.RubyObject = object.NIL
				for _, e := range x.Entries {
					if rubyEqual(e.Key, k) {
						found = e.Value
						break
					}
				}
				cur = found
			default:
				return object.NIL, nil
			}
			if _, isNil := cur.(*object.Nil); isNil {
				return object.NIL, nil
			}
		}
		return cur, nil
	case "each_with_index":
		// No-block form: return an Array of [elem, index] pairs.
		// Block form registered on ArrayClass via z_block_array.go.
		out := make([]object.RubyObject, len(r.Elements))
		for i, e := range r.Elements {
			out[i] = object.NewArray(e, object.NewInteger(int64(i)))
		}
		return object.NewArray(out...), nil
	case "each_slice":
		if len(args) != 1 {
			return nil, errorf("evaluator: Array#each_slice expects 1 arg")
		}
		n, ok := args[0].(*object.Integer)
		if !ok || n.Value <= 0 {
			return nil, errorf("evaluator: Array#each_slice needs positive Integer")
		}
		step := int(n.Value)
		out := []object.RubyObject{}
		for i := 0; i < len(r.Elements); i += step {
			end := i + step
			if end > len(r.Elements) {
				end = len(r.Elements)
			}
			slice := make([]object.RubyObject, end-i)
			copy(slice, r.Elements[i:end])
			out = append(out, object.NewArray(slice...))
		}
		return object.NewArray(out...), nil
	case "each_cons":
		if len(args) != 1 {
			return nil, errorf("evaluator: Array#each_cons expects 1 arg")
		}
		n, ok := args[0].(*object.Integer)
		if !ok || n.Value <= 0 {
			return nil, errorf("evaluator: Array#each_cons needs positive Integer")
		}
		w := int(n.Value)
		if w > len(r.Elements) {
			return object.NewArray(), nil
		}
		out := make([]object.RubyObject, 0, len(r.Elements)-w+1)
		for i := 0; i+w <= len(r.Elements); i++ {
			slice := make([]object.RubyObject, w)
			copy(slice, r.Elements[i:i+w])
			out = append(out, object.NewArray(slice...))
		}
		return object.NewArray(out...), nil
	case "zip":
		out := make([]object.RubyObject, len(r.Elements))
		others := make([]*object.Array, 0, len(args))
		for _, a := range args {
			oa, ok := a.(*object.Array)
			if !ok {
				return nil, errorf("evaluator: Array#zip needs Array args, got %T", a)
			}
			others = append(others, oa)
		}
		for i, e := range r.Elements {
			tuple := make([]object.RubyObject, 1+len(others))
			tuple[0] = e
			for j, o := range others {
				if i < len(o.Elements) {
					tuple[j+1] = o.Elements[i]
				} else {
					tuple[j+1] = object.NIL
				}
			}
			out[i] = object.NewArray(tuple...)
		}
		return object.NewArray(out...), nil
	case "take":
		if len(args) != 1 {
			return nil, errorf("evaluator: Array#take expects 1 arg")
		}
		n, ok := args[0].(*object.Integer)
		if !ok {
			return nil, errorf("evaluator: Array#take needs Integer")
		}
		cnt := int(n.Value)
		if cnt < 0 {
			return nil, errorf("evaluator: ArgumentError: negative array size")
		}
		if cnt > len(r.Elements) {
			cnt = len(r.Elements)
		}
		out := make([]object.RubyObject, cnt)
		copy(out, r.Elements[:cnt])
		return object.NewArray(out...), nil
	case "drop":
		if len(args) != 1 {
			return nil, errorf("evaluator: Array#drop expects 1 arg")
		}
		n, ok := args[0].(*object.Integer)
		if !ok {
			return nil, errorf("evaluator: Array#drop needs Integer")
		}
		cnt := int(n.Value)
		if cnt < 0 {
			return nil, errorf("evaluator: ArgumentError: negative array size")
		}
		if cnt > len(r.Elements) {
			cnt = len(r.Elements)
		}
		out := make([]object.RubyObject, len(r.Elements)-cnt)
		copy(out, r.Elements[cnt:])
		return object.NewArray(out...), nil
	case "join":
		sep := ""
		if len(args) == 1 {
			if t, ok := stringText(env, args[0]); ok {
				sep = t
			}
		}
		parts := make([]string, len(r.Elements))
		for i, e := range r.Elements {
			parts[i] = toStringValue(env, e)
		}
		return object.NewString(strings.Join(parts, sep)), nil
	case "min":
		if len(r.Elements) == 0 {
			return object.NIL, nil
		}
		best := r.Elements[0]
		for _, e := range r.Elements[1:] {
			c, ok := compareObjectsEnv(env, e, best)
			if !ok {
				return nil, errorf("evaluator: comparison failed in Array#min")
			}
			if c < 0 {
				best = e
			}
		}
		return best, nil
	case "minmax":
		if len(r.Elements) == 0 {
			return object.NewArray(object.NIL, object.NIL), nil
		}
		lo := r.Elements[0]
		hi := r.Elements[0]
		for _, e := range r.Elements[1:] {
			if c, ok := compareObjectsEnv(env, e, lo); ok && c < 0 {
				lo = e
			}
			if c, ok := compareObjectsEnv(env, e, hi); ok && c > 0 {
				hi = e
			}
		}
		return object.NewArray(lo, hi), nil
	case "max":
		if len(r.Elements) == 0 {
			return object.NIL, nil
		}
		best := r.Elements[0]
		for _, e := range r.Elements[1:] {
			c, ok := compareObjectsEnv(env, e, best)
			if !ok {
				return nil, errorf("evaluator: comparison failed in Array#max")
			}
			if c > 0 {
				best = e
			}
		}
		return best, nil
	case "sum":
		// Fast path: numeric-only with no init or init==0 -> direct
		// int/float accumulation. Anything else (init given, or any
		// non-numeric element) falls through to generic + dispatch so
		// `["a","b"].sum("")` and `[[1],[2]].sum([])` work.
		if len(args) == 0 {
			fast := true
			for _, e := range r.Elements {
				switch e.(type) {
				case *object.Integer, *object.Float:
				default:
					fast = false
				}
				if !fast {
					break
				}
			}
			if fast {
				var intSum int64
				var floatSum float64
				anyFloat := false
				for _, e := range r.Elements {
					switch v := e.(type) {
					case *object.Integer:
						if anyFloat {
							floatSum += float64(v.Value)
						} else {
							intSum += v.Value
						}
					case *object.Float:
						if !anyFloat {
							floatSum = float64(intSum)
							anyFloat = true
						}
						floatSum += v.Value
					}
				}
				if anyFloat {
					return object.NewFloat(floatSum), nil
				}
				return object.NewInteger(intSum), nil
			}
		}
		var acc object.RubyObject
		if len(args) == 1 {
			acc = args[0]
		} else {
			acc = object.NewInteger(0)
		}
		for _, e := range r.Elements {
			// String/Array concat fast paths -- mirrors evalInfix's `+`.
			if l, ok := acc.(*object.String); ok {
				if t, ok := stringText(env, e); ok {
					acc = object.NewString(string(l.Buf) + t)
					continue
				}
			}
			if la, ok := acc.(*object.Array); ok {
				if ra, ok := e.(*object.Array); ok {
					elems := make([]object.RubyObject, 0, len(la.Elements)+len(ra.Elements))
					elems = append(elems, la.Elements...)
					elems = append(elems, ra.Elements...)
					acc = object.NewArray(elems...)
					continue
				}
			}
			v, err := callMethod(env, acc, "+", []object.RubyObject{e})
			if err != nil {
				return nil, err
			}
			acc = v
		}
		return acc, nil
	case "grep":
		if len(args) != 1 {
			return nil, errorf("evaluator: Array#grep expects 1 arg, got %d", len(args))
		}
		pattern := args[0]
		out := []object.RubyObject{}
		for _, e := range r.Elements {
			// Use `===` on the pattern for primitives we know, or
			// dispatch the pattern's `===` method for instances /
			// modules. `caseEqual` already does this for built-in
			// patterns; for user-defined `===` we call directly.
			if inst, ok := pattern.(*object.Instance); ok {
				if _, found := dispatchClass(env, inst).LookupMethod("==="); found {
					v, err := callMethod(env, pattern, "===", []object.RubyObject{e})
					if err != nil {
						return nil, err
					}
					if truthy(v) {
						out = append(out, e)
					}
					continue
				}
			}
			if caseEqual(env, pattern, e) {
				out = append(out, e)
			}
		}
		return object.NewArray(out...), nil
	case "uniq":
		out := []object.RubyObject{}
	dedup:
		for _, e := range r.Elements {
			for _, k := range out {
				if rubyEqual(e, k) {
					continue dedup
				}
			}
			out = append(out, e)
		}
		return object.NewArray(out...), nil
	case "compact":
		out := []object.RubyObject{}
		for _, e := range r.Elements {
			if _, isNil := e.(*object.Nil); !isNil {
				out = append(out, e)
			}
		}
		return object.NewArray(out...), nil
	case "flatten":
		depth := -1
		if len(args) == 1 {
			d, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Array#flatten depth must be Integer")
			}
			depth = int(d.Value)
		}
		return object.NewArray(flattenArrayDepth(r, depth)...), nil
	case "reduce", "inject":
		// No-block forms: `reduce(:+)` and `reduce(init, :+)`.
		// Block forms are handled in callMethodWithBlock.
		var op string
		var acc object.RubyObject
		start := 0
		switch {
		case len(args) == 1:
			sym, ok := args[0].(*object.Symbol)
			if !ok {
				return nil, errorf("evaluator: Array#reduce(sym) needs Symbol")
			}
			op = env.Symbols().Name(sym.ID)
			if len(r.Elements) == 0 {
				return object.NIL, nil
			}
			acc = r.Elements[0]
			start = 1
		case len(args) == 2:
			sym, ok := args[1].(*object.Symbol)
			if !ok {
				return nil, errorf("evaluator: Array#reduce(init, sym) needs Symbol")
			}
			op = env.Symbols().Name(sym.ID)
			acc = args[0]
		default:
			return nil, errorf("evaluator: Array#reduce: wrong number of arguments (%d)", len(args))
		}
		for i := start; i < len(r.Elements); i++ {
			v, err := evalBinaryOp(env, op, acc, r.Elements[i])
			if err != nil {
				return nil, err
			}
			acc = v
		}
		return acc, nil
	case "tally":
		entries := []object.HashEntry{}
		for _, e := range r.Elements {
			found := false
			for i := range entries {
				if rubyEqual(entries[i].Key, e) {
					v := entries[i].Value.(*object.Integer)
					entries[i].Value = object.NewInteger(v.Value + 1)
					found = true
					break
				}
			}
			if !found {
				entries = append(entries, object.HashEntry{Key: e, Value: object.NewInteger(1)})
			}
		}
		return object.NewHash(entries...), nil
	}
	return raiseBuiltin(env, "NoMethodError", "undefined method `"+name+"' for Array")
}

// arrayOfArrays wraps a Go [][]RubyObject into a Ruby Array of Arrays
// for combination / permutation results.
func arrayOfArrays(rows [][]object.RubyObject) *object.Array {
	out := make([]object.RubyObject, len(rows))
	for i, row := range rows {
		out[i] = object.NewArray(row...)
	}
	return object.NewArray(out...)
}

// combinations enumerates all unordered k-element subsets of elems in
// MRI's lexicographic order (preserves elems' relative order). k <= 0
// returns a single empty subset; k > len(elems) returns no subsets.
func combinations(elems []object.RubyObject, k int) [][]object.RubyObject {
	if k < 0 {
		return nil
	}
	if k == 0 {
		return [][]object.RubyObject{{}}
	}
	if k > len(elems) {
		return nil
	}
	out := [][]object.RubyObject{}
	cur := make([]object.RubyObject, 0, k)
	var rec func(start int)
	rec = func(start int) {
		if len(cur) == k {
			cp := make([]object.RubyObject, k)
			copy(cp, cur)
			out = append(out, cp)
			return
		}
		for i := start; i < len(elems); i++ {
			cur = append(cur, elems[i])
			rec(i + 1)
			cur = cur[:len(cur)-1]
		}
	}
	rec(0)
	return out
}

// permutations enumerates all ordered k-element arrangements of elems
// in MRI's order (positions iterated left-to-right over remaining
// candidates). k <= 0 returns the single empty arrangement; k > len
// returns no arrangements.
func permutations(elems []object.RubyObject, k int) [][]object.RubyObject {
	if k < 0 {
		return nil
	}
	if k == 0 {
		return [][]object.RubyObject{{}}
	}
	if k > len(elems) {
		return nil
	}
	out := [][]object.RubyObject{}
	cur := make([]object.RubyObject, 0, k)
	used := make([]bool, len(elems))
	var rec func()
	rec = func() {
		if len(cur) == k {
			cp := make([]object.RubyObject, k)
			copy(cp, cur)
			out = append(out, cp)
			return
		}
		for i := range elems {
			if used[i] {
				continue
			}
			used[i] = true
			cur = append(cur, elems[i])
			rec()
			cur = cur[:len(cur)-1]
			used[i] = false
		}
	}
	rec()
	return out
}
