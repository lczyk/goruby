package object

import (
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/lczyk/goruby/trace"
)

var rangeClass RubyClassObject = newClass(
	"Range",
	rangeMethods,
	nil,
	notInstantiatable,
)

func init() {
	CLASSES.Set("Range", rangeClass)
}

type Range struct {
	Left      int64
	Right     int64
	Inclusive bool
}

func (a *Range) Inspect() string {
	var out strings.Builder
	out.WriteString(fmt.Sprintf("%d", a.Left))
	if a.Inclusive {
		out.WriteString("..")
	} else {
		out.WriteString("...")
	}
	out.WriteString(fmt.Sprintf("%d", a.Right))
	return out.String()
}

func (a *Range) Class() RubyClass { return rangeClass }

func (a *Range) HashKey() HashKey {
	h := fnv.New64a()
	h.Write([]byte(fmt.Sprintf("%d", a.Left)))
	h.Write([]byte(fmt.Sprintf("%d", a.Right)))
	if a.Inclusive {
		h.Write([]byte("1"))
	} else {
		h.Write([]byte("0"))
	}
	return HashKey(h.Sum64())
}

var rangeMethods = map[string]RubyMethod{
	"find_all": withArity(1, newMethod(rangeFindAll)),
	"all?":     newMethod(rangeAll),
	"size":     newMethod(rangeSize),
}

// Actually create an array of integers from the range
func (rang *Range) ToArray() *Array {
	result := NewArray()
	left, right := rang.Left, rang.Right
	if rang.Inclusive {
		right++
	}
	for i := left; i < right; i++ {
		result.Elements = append(result.Elements, NewInteger(i))
	}
	return result
}

func rangeFindAll(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	rng, _ := context.Receiver().(*Range)
	proc, ok := args[0].(*Symbol)
	if !ok {
		return nil, NewArgumentError("(2) range find_all requires a block")
	}
	// self, _ := context.Env().Get( "funcs")
	// self_class := self.Class()
	fn, ok := FUNCS_STORE.Class().GetMethod(proc.Value)
	if !ok {
		return nil, NewNoMethodError(FUNCS_STORE, proc.Value)
	}
	// evaluate the range
	result := NewArray()
	for _, elem := range rng.ToArray().Elements {
		ret, err := fn.Call(context, tracer, elem)
		if err != nil {
			return nil, err
		}
		if ret == nil {
			return nil, NewArgumentError("find_all requires a block to return a boolean")
		}
		val, ok := SymbolToBool(ret)
		if !ok {
			return nil, NewArgumentError("find_all requires a block to return a boolean")
		}
		if val {
			result.Elements = append(result.Elements, elem)
		}
	}
	return result, nil
}

func rangeAll(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	rng, _ := context.Receiver().(*Range)
	if len(args) == 0 {
		return nil, NewArgumentError("all? requires a block")
	}
	block := args[0]
	proc, ok := block.(*Symbol)
	if !ok {
		return nil, NewArgumentError("all? requires a block")
	}
	fn, ok := FUNCS_STORE.GetMethod(proc.Value)
	if !ok {
		return nil, NewNoMethodError(FUNCS_STORE, proc.Value)
	}
	for _, elem := range rng.ToArray().Elements {
		ret, err := fn.Call(context, tracer, elem)
		if err != nil {
			return nil, err
		}
		val, ok := SymbolToBool(ret)
		if !ok {
			return nil, NewArgumentError("all? requires a block to return a boolean")
		}
		if !val {
			return FALSE, nil
		}
	}
	return TRUE, nil
}

func rangeSize(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	rng, _ := context.Receiver().(*Range)
	size := rng.Right - rng.Left
	if rng.Inclusive {
		size++
	}
	return NewInteger(size), nil
}
