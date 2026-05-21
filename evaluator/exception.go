package evaluator

import (
	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/object"
)

// errZeroDivision is the typed sentinel emitted by integer division
// when the divisor is zero. evalInfix translates it into a raised
// ZeroDivisionError ruby exception once env is in reach.
type zeroDivErr struct{}

func (zeroDivErr) Error() string { return "ZeroDivisionError: divided by 0" }

var errZeroDivision = zeroDivErr{}

// raiseSignal carries a ruby Exception instance out through the
// evaluator's error channel until something rescues it.
type raiseSignal struct {
	Exception *object.Instance
}

func (r *raiseSignal) Error() string {
	if r.Exception == nil {
		return "raised: nil"
	}
	if msg, ok := r.Exception.Ivars["@message"]; ok {
		if s, ok := msg.(*object.String); ok {
			return "raised: " + r.Exception.C.Name + ": " + string(s.Buf)
		}
	}
	return "raised: " + r.Exception.C.Name
}

// bootstrapExceptionHierarchy installs the built-in exception classes
// the corpus needs. Idempotent: subsequent calls find them on env.
func bootstrapExceptionHierarchy(env *object.Environment) {
	if _, ok := env.Get("Exception"); ok {
		return
	}
	root := object.NewClass("Exception", nil)
	root.Methods["message"] = &object.UserMethod{Name: "message", Body: exceptionMessageMarker{}}
	root.Methods["to_s"] = &object.UserMethod{Name: "to_s", Body: exceptionMessageMarker{}}
	root.Methods["initialize"] = &object.UserMethod{Name: "initialize", Body: exceptionInitMarker{}}
	env.SetGlobal("Exception", root)

	for _, def := range builtinExceptionTree {
		super, _ := env.Get(def.super)
		c := object.NewClass(def.name, super.(*object.Class))
		env.SetGlobal(def.name, c)
	}
}

// exceptionMessageMarker is the body sentinel for the synthesised
// Exception#message accessor (reads @message).
type exceptionMessageMarker struct{}

// exceptionInitMarker is the body sentinel for Exception#initialize.
// Stores the first arg's String form as @message (or the class name
// when no arg is given).
type exceptionInitMarker struct{}

var builtinExceptionTree = []struct {
	name  string
	super string
}{
	{"StandardError", "Exception"},
	{"RuntimeError", "StandardError"},
	{"ArgumentError", "StandardError"},
	{"TypeError", "StandardError"},
	{"NameError", "StandardError"},
	{"NoMethodError", "NameError"},
	{"ZeroDivisionError", "StandardError"},
	{"IndexError", "StandardError"},
	{"KeyError", "StandardError"},
	{"StopIteration", "IndexError"},
	{"LocalJumpError", "StandardError"},
	{"NotImplementedError", "StandardError"},
	{"RangeError", "StandardError"},
	{"FloatDomainError", "RangeError"},
	{"LoadError", "StandardError"},
	{"FrozenError", "RuntimeError"},
}

// kernelRaise implements `raise`, in its three argument shapes.
func kernelRaise(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	bootstrapExceptionHierarchy(env)
	exc, err := buildRaisedException(env, args)
	if err != nil {
		return nil, err
	}
	return nil, &raiseSignal{Exception: exc}
}

func buildRaisedException(env *object.Environment, args []object.RubyObject) (*object.Instance, error) {
	runtimeErr, _ := env.Get("RuntimeError")
	runtimeClass := runtimeErr.(*object.Class)

	switch len(args) {
	case 0:
		return newExceptionInstance(runtimeClass, ""), nil
	case 1:
		switch v := args[0].(type) {
		case *object.Instance:
			return v, nil
		case *object.Class:
			return newExceptionInstance(v, v.Name), nil
		case *object.String:
			return newExceptionInstance(runtimeClass, string(v.Buf)), nil
		case *object.FrozenString:
			return newExceptionInstance(runtimeClass, env.Strings().Get(v.ID)), nil
		}
		return nil, errorf("evaluator: TypeError: exception class/object expected, got %T", args[0])
	case 2:
		cls, ok := args[0].(*object.Class)
		if !ok {
			return nil, errorf("evaluator: TypeError: exception class expected, got %T", args[0])
		}
		msg, ok := stringText(env, args[1])
		if !ok {
			return nil, errorf("evaluator: TypeError: exception message must be String, got %T", args[1])
		}
		return newExceptionInstance(cls, msg), nil
	}
	return nil, errorf("evaluator: ArgumentError: wrong number of arguments to raise (given %d)", len(args))
}

func newExceptionInstance(cls *object.Class, msg string) *object.Instance {
	inst := object.NewInstance(cls)
	inst.Ivars["@message"] = object.NewString(msg)
	return inst
}

// evalExceptionHandlingBlock implements begin/rescue/else/ensure. The
// match strategy: a rescue with no ExceptionClasses catches any
// StandardError descendant; otherwise the raised exception's class
// must descend from one of the listed classes.
func evalExceptionHandlingBlock(env *object.Environment, n *ast.ExceptionHandlingBlock) (object.RubyObject, error) {
	bootstrapExceptionHierarchy(env)

	result, err := evalBlockStatement(env, n.TryBody)

	if err == nil {
		if n.ElseBody != nil {
			result, err = evalBlockStatement(env, n.ElseBody)
		}
	}

	rs, raised := err.(*raiseSignal)
	if err != nil && raised {
		handled := false
		for _, r := range n.Rescues {
			match, mErr := rescueMatches(env, r, rs.Exception)
			if mErr != nil {
				err = mErr
				break
			}
			if !match {
				continue
			}
			if r.Exception != nil {
				env.AssignVisible(r.Exception.Value, rs.Exception)
			}
			result, err = evalBlockStatement(env, r.Body)
			handled = true
			break
		}
		if !handled {
			err = rs
		}
	}

	if n.EnsureBody != nil {
		if _, eerr := evalBlockStatement(env, n.EnsureBody); eerr != nil {
			err = eerr
		}
	}

	return result, err
}

// raiseBuiltin constructs and raises an exception of the named
// built-in class with the given message. Convenience for evaluator
// sites that need to raise without an explicit `raise` call.
func raiseBuiltin(env *object.Environment, className, msg string) (object.RubyObject, error) {
	bootstrapExceptionHierarchy(env)
	cls, ok := env.Get(className)
	if !ok {
		return nil, errorf("evaluator: unknown built-in exception class %s", className)
	}
	c, ok := cls.(*object.Class)
	if !ok {
		return nil, errorf("evaluator: %s is not a class", className)
	}
	return nil, &raiseSignal{Exception: newExceptionInstance(c, msg)}
}

func rescueMatches(env *object.Environment, r *ast.RescueBlock, exc *object.Instance) (bool, error) {
	if len(r.ExceptionClasses) == 0 {
		stdErr, _ := env.Get("StandardError")
		base, ok := stdErr.(*object.Class)
		if !ok {
			return true, nil
		}
		return exc.C.IsAncestor(base), nil
	}
	for _, c := range r.ExceptionClasses {
		obj, err := Eval(c, env)
		if err != nil {
			return false, err
		}
		cls, ok := obj.(*object.Class)
		if !ok {
			return false, errorf("evaluator: TypeError: class or module required for rescue clause, got %T", obj)
		}
		if exc.C.IsAncestor(cls) {
			return true, nil
		}
	}
	return false, nil
}
