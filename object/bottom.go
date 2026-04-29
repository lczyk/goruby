package object

import (
	"fmt"
	"strings"

	"github.com/lczyk/goruby/trace"
)

var (
	nil_class   *class          = nil
	bottomClass *class          = nil_class
	BOTTOM      *extendedObject = nil
)

// TODO: make sure we don't collide with other hash keys
const HASH_KEY_BOTTOM HashKey = 0

func init() {
	// NOTE: create the bottom class in init to avoid circular import
	bottomClass = newClass(
		"Bottom",
		bottomMethodSet,
		nil,
		notInstantiatable, // not instantiatable through new
	)
	bottomClass.class = bottomClass
	CLASSES.Set("Bottom", bottomClass)

	BOTTOM = newExtendedObject(&Bottom{})
}

type Bottom struct{}

func (o *Bottom) Inspect() string  { return "" }
func (o *Bottom) Class() RubyClass { return bottomClass }
func (o *Bottom) HashKey() HashKey { return HASH_KEY_BOTTOM }

var bottomMethodSet = map[string]RubyMethod{
	"to_s":    withArity(0, newMethod(bottomToS)),
	"is_a?":   withArity(1, newMethod(bottomIsA)),
	"nil?":    withArity(0, newMethod(bottomIsNil)),
	"methods": newMethod(bottomMethods),
	"class":   withArity(0, newMethod(bottomClassMethod)),
	"puts":    newMethod(bottomPuts),
	"print":   newMethod(bottomPrint),
	"raise":   newMethod(bottomRaise),
	"==":      withArity(1, newMethod(bottomEqual)),
	"!=":      withArity(1, newMethod(bottomNotEqual)),
}

func bottomToS(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	receiver := context.Receiver()
	val := fmt.Sprintf("#<%s:%p>", receiver.Class().Name(), receiver)
	return NewString(val), nil
}

func bottomIsA(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	receiver_class := context.Receiver().Class()
	switch arg := args[0].(type) {
	case RubyClassObject:
		if arg.Name() == receiver_class.Name() {
			return TRUE, nil
		} else {
			return FALSE, nil
		}
	default:
		return nil, NewTypeError("argument must be a Class")
	}
}

func print(lines []string, delimiter string) {
	var out strings.Builder
	for i, line := range lines {
		out.WriteString(line)
		if i != len(lines)-1 {
			out.WriteString(delimiter)
		}
	}
	out.WriteString(delimiter)
	fmt.Print(out.String())
}

func bottomPuts(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	var lines []string
	for _, arg := range args {
		if arr, ok := arg.(*Array); ok {
			// arg is an array. splat it out
			// todo: make it a deep splat? check with original ruby implementation
			for _, elem := range arr.Elements {
				lines = append(lines, elem.Inspect())
			}
		} else {
			switch arg := arg.(type) {
			case *Symbol:
				if arg == NIL {
					//
				} else {
					lines = append(lines, arg.Inspect())
				}
			default:
				lines = append(lines, arg.Inspect())
			}
		}
	}
	print(lines, "\n")
	return NIL, nil
}

func bottomPrint(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	var lines []string
	for _, arg := range args {
		if arr, ok := arg.(*Array); ok {
			// arg is an array. splat it out
			// todo: make it a deep splat? check with original ruby implementation
			// for _, elem := range arr.Elements {
			// 	lines = append(lines, elem.Inspect())
			// }
			lines = append(lines, arr.Inspect())
		} else {
			switch arg := arg.(type) {
			case *Symbol:
				if arg == NIL {
					//
				} else {
					lines = append(lines, arg.Inspect())
				}
			default:
				lines = append(lines, arg.Inspect())
			}
		}
	}
	print(lines, "")
	return NIL, nil
}

func bottomMethods(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	showSuperMethods := true
	if len(args) == 1 {
		if val, ok := SymbolToBool(args[0]); ok {
			showSuperMethods = val
		}
	}

	receiver := context.Receiver()
	class := context.Receiver().Class()

	extended, ok := receiver.(*extendedObject)

	if !showSuperMethods && !ok {
		return &Array{}, nil
	}

	if !showSuperMethods && ok {
		class = extended.eigenclass
	}

	return getMethods(class, showSuperMethods), nil
}

func bottomIsNil(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	receiver := context.Receiver()
	if receiver == NIL {
		return TRUE, nil
	}
	return FALSE, nil
}

func bottomClassMethod(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	receiver := context.Receiver()
	if _, ok := receiver.(RubyClassObject); ok {
		return nil, nil
	}
	return receiver.Class().(RubyClassObject), nil
}

func bottomRaise(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	switch len(args) {
	case 1:
		switch arg := args[0].(type) {
		case *String:
			return nil, NewRuntimeError("%s", arg.Value)
		default:
			return nil, NewRuntimeError("%s", arg.Inspect())
			// default:
			// 	exc, err := Send(NewCallContext(context.Env(), arg), "exception")
			// 	if err != nil {
			// 		return nil, NewTypeError("exception class/object expected")
			// 	}
			// 	if excAsErr, ok := exc.(error); ok {
			// 		return nil, excAsErr
			// 	}
			// 	return nil, nil
		}
	default:
		return nil, NewRuntimeError("")
	}
}

func swapOrFalse(left, right RubyObject, swapped bool) bool {
	if swapped {
		// we've already swapped. just return false
		return false
	} else {
		// 1-depth recursive call with swapped arguments
		return rubyObjectsEqual(right, left, true)
	}
}

// TODO: Unify this with rubyObjectsEqual
func CompareRubyObjectsForTests(a, b any) bool {
	// check nils
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// check types
	a_obj, a_ok := a.(RubyObject)
	// if !ok {
	// 	panic("a is not RubyObject")
	// }
	b_obj, b_ok := b.(RubyObject)
	// if !ok {
	// 	panic("b is not RubyObject")
	// }
	if !a_ok || !b_ok {
		// maybe we're both arrays of RubyObjects?
		a_arr, a_ok := a.([]RubyObject)
		b_arr, b_ok := b.([]RubyObject)
		if a_ok && b_ok {
			// compare the arrays element by element
			if len(a_arr) != len(b_arr) {
				return false
			}
			for i := range a_arr {
				if !CompareRubyObjectsForTests(a_arr[i], b_arr[i]) {
					return false
				}
			}
			return true
		} else {
			if !a_ok {
				panic("a is not RubyObject or []RubyObject")
			}
			if !b_ok {
				panic("b is not RubyObject or []RubyObject")
			}
			panic("a and b are not RubyObject or []RubyObject")
		}
	}

	if a_obj.Class() != b_obj.Class() {
		return false
	}
	// TODO: look into more
	return a_obj.HashKey() == b_obj.HashKey()
	// if a, a_hashable := a_obj.(hashable); a_hashable {
	// 	if b, b_hashable := b_obj.(hashable); b_hashable {
	// 	} else {
	// 		// b is not hashable, we are not equal
	// 		return false
	// 	}
	// }
	// if _, b_hashable := b_obj.(hashable); b_hashable {
	// 	// a is not hashable, we are not equal
	// 	return false
	// }
	// ok, we are not hashable but we are the same class
	// check the addresses

	// addrB := fmt.Sprintf("%p", b_obj)
	// if addrA == addrB {
	// 	return true
	// }
	// return reflect.DeepEqual(a_obj, b_obj)
}

func rubyObjectsEqual(left, right RubyObject, swapped bool) bool {
	// leftClass := left.Class()
	// rightClass := right.Class()
	// if leftClass != rightClass {
	// 	return swapOrFalse(left, right, swapped)
	// }
	// if left == nil {
	// 	return right == nil || right.Class().Name() == "NilClass"
	// }
	// if left.Class().Name() == "NilClass" {
	// 	return right == nil || right.Class().Name() == "NilClass"
	// }
	// fmt.Printf("left: %T right: %T\n", left, right)
	// fmt.Println("left:", left.Class().Name(), "right:", right.Class().Name())
	switch left := left.(type) {
	case *Integer:
		right_t, ok := safeObjectToInteger(right)
		if !ok {
			return swapOrFalse(left, right, swapped)
		} else {
			return left.Value == right_t
		}
	case *Float:
		right_t, ok := safeObjectToFloat(right)
		if !ok {
			return swapOrFalse(left, right, swapped)
		} else {
			return left.Value == right_t
		}
	case *String:
		if right_t, ok := right.(*String); !ok {
			return swapOrFalse(left, right, swapped)
		} else {
			return left.Value == right_t.Value
		}
	case *Array:
		if right_t, ok := right.(*Array); !ok {
			return swapOrFalse(left, right, swapped)
		} else {
			if len(left.Elements) != len(right_t.Elements) {
				return false
			}
			for i, elem := range left.Elements {
				if !rubyObjectsEqual(elem, right_t.Elements[i], swapped) {
					return false
				}
			}
			return true
		}
	case *Hash:
		if right_t, ok := right.(*Hash); !ok {
			return swapOrFalse(left, right, swapped)
		} else {
			if len(left.Map) != len(right_t.Map) {
				return false
			}
			for key, leftValue := range left.ObjectMap() {
				rightValue, ok := right_t.Get(key)
				if !ok {
					return false
				}
				if !rubyObjectsEqual(leftValue, rightValue, swapped) {
					return false
				}
			}

			return true
		}
	case *Symbol:
		if right_t, ok := right.(*Symbol); !ok {
			return swapOrFalse(left, right, swapped)
		} else {
			return left.Value == right_t.Value
		}
	default:
		return false
	}
}

func RubyObjectsEqual(left, right RubyObject) bool {
	return rubyObjectsEqual(left, right, false)
}

func bottomEqual(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	if RubyObjectsEqual(context.Receiver(), args[0]) {
		return TRUE, nil
	}
	return FALSE, nil
}

func bottomNotEqual(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	if RubyObjectsEqual(context.Receiver(), args[0]) {
		return FALSE, nil
	}
	return TRUE, nil
}
