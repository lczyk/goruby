package object

import (
	"hash/fnv"
	"strings"

	"github.com/lczyk/goruby/trace"
)

var arrayClass RubyClassObject = newClass(
	"Array",
	arrayMethods,
	nil,
	func(c RubyClassObject, args ...RubyObject) (RubyObject, error) { return NewArray(args...), nil },
)

func init() {
	CLASSES.Set("Array", arrayClass)
}

// NewArray returns a new array populated with elements.
func NewArray(elements ...RubyObject) *Array {
	arr := &Array{Elements: make([]RubyObject, len(elements))}
	if len(elements) == 0 {
		return arr
	}
	copy(arr.Elements, elements)
	return arr
}

type Array struct {
	Elements []RubyObject
}

func (a *Array) Inspect() string {
	elems := make([]string, len(a.Elements))
	for i, elem := range a.Elements {
		var elem_str string
		switch elem.(type) {
		case *String:
			elem_str = "\"" + elem.Inspect() + "\""
		default:
			elem_str = elem.Inspect()
		}
		elems[i] = elem_str
	}
	return "[" + strings.Join(elems, ", ") + "]"
}

func (a *Array) Class() RubyClass { return arrayClass }
func (a *Array) HashKey() HashKey {
	h := fnv.New64a()
	for _, e := range a.Elements {
		h.Write(e.HashKey().bytes())
	}
	return HashKey(h.Sum64())
}

var arrayMethods = map[string]RubyMethod{
	"push":     newMethod(arrayPush),
	"unshift":  newMethod(arrayUnshift),
	"size":     newMethod(arraySize),
	"length":   newMethod(arraySize),
	"find_all": newMethod(arrayFindAll),
	"first":    newMethod(arrayFirst),
	"map":      newMethod(arrayMap),
	"all?":     newMethod(arrayAll),
	"join":     newMethod(arrayJoin),
	"include?": newMethod(arrayInclude),
	"each":     newMethod(arrayEach),
	"reject":   newMethod(arrayReject),
	"pop":      newMethod(arrayPop),
	"-":        newMethod(arrayMinus),
	"+":        newMethod(arrayPlus),
	"*":        newMethod(arrayAst),
}

func arrayPush(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	array, _ := context.Receiver().(*Array)
	array.Elements = append(array.Elements, args...)
	return array, nil
}

func arrayUnshift(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	array, _ := context.Receiver().(*Array)
	array.Elements = append(args, array.Elements...)
	return array, nil
}

func arraySize(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	array, _ := context.Receiver().(*Array)
	return NewInteger(int64(len(array.Elements))), nil
}

func arrayFindAll(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	array, _ := context.Receiver().(*Array)
	if len(args) == 0 {
		return nil, NewArgumentError("(1) array find_all requires a block")
	}
	block := args[0]
	proc, ok := block.(*Symbol)
	if !ok {
		return nil, NewArgumentError("(2) array find_all requires a block")
	}
	fn, ok := FUNCS_STORE.GetMethod(proc.Value)
	if !ok {
		return nil, NewNoMethodError(FUNCS_STORE, proc.Value)
	}
	result := NewArray()
	for _, element := range array.Elements {
		ret, err := fn.Call(context, tracer, element)
		if err != nil {
			return nil, err
		}
		if ret == nil {
			return nil, NewArgumentError("find_all requires a block to return a boolean, not nil")
		}
		val, ok := SymbolToBool(ret)
		if !ok {
			return nil, NewArgumentError("find_all requires a block to return a boolean, not %s", ret.Inspect())
		}
		if val {
			result.Elements = append(result.Elements, element)
		}
	}
	return result, nil
}

func arrayFirst(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	array, _ := context.Receiver().(*Array)
	if len(args) == 0 {
		if len(array.Elements) == 0 {
			return nil, NewArgumentError("array is empty")
		}
		return array.Elements[0], nil
	}
	if len(args) > 1 {
		return nil, NewArgumentError("wrong number of arguments (given %d, expected 0..1)", len(args))
	}
	n, ok := args[0].(*Integer)
	if !ok {
		return nil, NewArgumentError("argument must be an Integer")
	}
	if n.Value < 0 {
		return nil, NewArgumentError("negative array size (or size too big)")
	}
	if int(n.Value) > len(array.Elements) {
		return nil, NewArgumentError("array size too big")
	}
	result := NewArray()
	for i := 0; i < int(n.Value); i++ {
		result.Elements = append(result.Elements, array.Elements[i])
	}
	return result, nil
}

func arrayMap(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
		tracer.Message(context.Receiver().Inspect())
	}
	array, _ := context.Receiver().(*Array)
	if len(args) == 0 {
		return nil, NewArgumentError("map requires a block")
	}
	block := args[0]
	proc, ok := block.(*Symbol)
	if !ok {
		return nil, NewArgumentError("map requires a block")
	}
	fn, ok := FUNCS_STORE.GetMethod(proc.Value)
	if !ok {
		return nil, NewNoMethodError(FUNCS_STORE, proc.Value)
	}
	result := NewArray()
	for _, elem := range array.Elements {
		ret, err := fn.Call(context, tracer, elem)
		if err != nil {
			return nil, err
		}
		result.Elements = append(result.Elements, ret)
	}
	return result, nil
}

func arrayAll(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	array, _ := context.Receiver().(*Array)
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
	for _, elem := range array.Elements {
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

func arrayJoin(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	array, _ := context.Receiver().(*Array)
	if len(args) == 0 {
		return nil, NewArgumentError("join requires at least 1 argument")
	}
	separator, ok := args[0].(*String)
	if !ok {
		return nil, NewArgumentError("argument must be a String")
	}
	element_strings := make([]string, len(array.Elements))
	for i, elem := range array.Elements {
		element_strings[i] = elem.Inspect()
	}
	result := strings.Join(element_strings, separator.Value)
	return NewString(result), nil
}

func arrayInclude(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	array, _ := context.Receiver().(*Array)
	if len(args) == 0 {
		return nil, NewArgumentError("include? requires at least 1 argument")
	}
	// compare the first argument with all elements in the array
	arg := args[0]
	for _, elem := range array.Elements {
		// if elem.Class() == arg.Class() {
		// 	if elem.Inspect() == arg.Inspect() {
		// 		return TRUE, nil
		// 	}
		// 	return FALSE, nil
		// } else {
		// 	if _, ok := elem.(*Integer); ok {
		// 		// also compare as if it was a float

		// 	}
		// }
		ctx := NewCallContext(context.Env(), elem)
		ret, err := Send(ctx, "==", nil, arg)
		if err != nil {
			// fmt.Println("Error in arrayInclude:", err)
			continue
			// return nil, err
		}
		val, ok := SymbolToBool(ret)
		if !ok {
			return nil, NewArgumentError("include? requires a block to return a boolean")
		}
		if val {
			return TRUE, nil
		}
	}
	return FALSE, nil
}

func arrayEach(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	array, _ := context.Receiver().(*Array)
	if len(args) == 0 {
		return nil, NewArgumentError("map requires a block")
	}
	block := args[0]
	proc, ok := block.(*Symbol)
	if !ok {
		return nil, NewArgumentError("map requires a block")
	}
	fn, ok := FUNCS_STORE.GetMethod(proc.Value)
	if !ok {
		return nil, NewNoMethodError(FUNCS_STORE, proc.Value)
	}
	for _, elem := range array.Elements {
		_, err := fn.Call(context, tracer, elem)
		if err != nil {
			return nil, err
		}
	}
	return array, nil
}

func arrayReject(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	array, _ := context.Receiver().(*Array)
	if len(args) == 0 {
		return nil, NewArgumentError("map requires a block")
	}
	block := args[0]
	proc, ok := block.(*Symbol)
	if !ok {
		return nil, NewArgumentError("map requires a block")
	}
	fn, ok := FUNCS_STORE.GetMethod(proc.Value)
	if !ok {
		return nil, NewNoMethodError(FUNCS_STORE, proc.Value)
	}
	result := NewArray()
	for _, elem := range array.Elements {
		ret, err := fn.Call(context, tracer, elem)
		if err != nil {
			return nil, err
		}
		if ret == nil {
			return nil, NewArgumentError("map requires a block to return a boolean")
		}
		val, ok := SymbolToBool(ret)
		if !ok {
			return nil, NewArgumentError("map requires a block to return a boolean")
		}
		if !val {
			result.Elements = append(result.Elements, elem)
		}
	}
	return result, nil
}

func arrayPop(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	array, _ := context.Receiver().(*Array)
	if len(array.Elements) == 0 {
		return NIL, nil
	}
	elem := array.Elements[len(array.Elements)-1]
	array.Elements = array.Elements[:len(array.Elements)-1]
	return elem, nil
}

func arrayMinus(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	array, _ := context.Receiver().(*Array)
	if len(args) == 0 {
		return nil, NewArgumentError("array minus requires at least 1 argument")
	}
	otherArray, ok := args[0].(*Array)
	if !ok {
		return nil, NewArgumentError("argument must be an Array")
	}
	result := NewArray()
	for _, elem := range array.Elements {
		include := false
		for _, otherElem := range otherArray.Elements {
			ctx := NewCallContext(context.Env(), elem)
			ret, err := Send(ctx, "==", nil, otherElem)
			if err != nil {
				return nil, err
			}
			val, ok := SymbolToBool(ret)
			if !ok {
				return nil, NewArgumentError("array minus requires a block to return a boolean")
			}
			if val {
				include = true
				break
			}
		}
		if !include {
			result.Elements = append(result.Elements, elem)
		}
	}
	return result, nil
}

func arrayPlus(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	array, _ := context.Receiver().(*Array)
	if len(args) == 0 {
		return nil, NewArgumentError("array plus requires at least 1 argument")
	}
	otherArray, ok := args[0].(*Array)
	if !ok {
		return nil, NewArgumentError("argument must be an Array")
	}
	result := NewArray()
	result.Elements = append(result.Elements, array.Elements...)
	result.Elements = append(result.Elements, otherArray.Elements...)
	return result, nil
}

func arrayAst(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	array, _ := context.Receiver().(*Array)
	if len(args) == 0 {
		return nil, NewArgumentError("array ast requires at least 1 argument")
	}
	switch arg := args[0].(type) {
	case *Integer:
		// repeat the array n times
		if arg.Value < 0 {
			return nil, NewArgumentError("negative array size (or size too big)")
		}
		// repeat the array n times
		result := NewArray()
		for i := int64(0); i < arg.Value; i++ {
			result.Elements = append(result.Elements, array.Elements...)
		}
		// fmt.Println("arrayAst", result.Inspect())
		return result, nil
	case *Float:
		// repeat the array floor(n) times
		if arg.Value < 0 {
			return nil, NewArgumentError("negative array size (or size too big)")
		}
		times := int64(arg.Value)
		result := NewArray()
		for i := int64(0); i < times; i++ {
			result.Elements = append(result.Elements, array.Elements...)
		}
		return result, nil
	case *String:
		// [1, 2,3] * ',' id the same as [1, 2,3].join(',')

		joiner := arg.Value
		element_strings := make([]string, len(array.Elements))
		for i, elem := range array.Elements {
			element_strings[i] = elem.Inspect()
		}
		result := strings.Join(element_strings, joiner)
		return NewString(result), nil

	default:
		return nil, NewArgumentError("argument must be an Integer, or a String")
	}
}
