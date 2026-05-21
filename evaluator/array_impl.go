package evaluator

import (
	"math/rand"
	"strings"

	"github.com/lczyk/goruby/object"
)

// randomIntn returns a non-negative pseudo-random int in [0, n). Used
// by Array#sample. Centralised so future seeding work (Kernel#srand,
// Random.new) has a single hook.
func randomIntn(n int) int {
	if n <= 0 {
		return 0
	}
	return rand.Intn(n)
}

func callArrayMethod(env *object.Environment, r *object.Array, name string, args []object.RubyObject) (object.RubyObject, error) {
	switch name {
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
	case "sort":
		out := make([]object.RubyObject, len(r.Elements))
		copy(out, r.Elements)
		if err := sortArray(env, out); err != nil {
			return nil, err
		}
		return object.NewArray(out...), nil
	case "include?":
		if len(args) != 1 {
			return nil, errorf("evaluator: wrong number of arguments to Array#include? (given %d, expected 1)", len(args))
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
		for _, e := range r.Elements {
			if truthy(e) {
				return object.TRUE, nil
			}
		}
		return object.FALSE, nil
	case "all?":
		for _, e := range r.Elements {
			if !truthy(e) {
				return object.FALSE, nil
			}
		}
		return object.TRUE, nil
	case "none?":
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
			default:
				return nil, errorf("evaluator: Array#sum: non-numeric element %T not supported", e)
			}
		}
		if anyFloat {
			return object.NewFloat(floatSum), nil
		}
		return object.NewInteger(intSum), nil
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
	return nil, errorf("evaluator: NoMethodError: undefined method `%s' for Array", name)
}
