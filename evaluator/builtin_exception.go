package evaluator

import (
	"strings"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/evaluator/builtinapi"
	"github.com/lczyk/goruby/object"
)

// init wires the builtinapi.RaiseBuiltin function-pointer so
// subpackages (evaluator/stdlib) can raise built-in exceptions
// without importing the evaluator package's raiseSignal type.
func init() {
	builtinapi.RaiseBuiltin = raiseBuiltin
}

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
	root.Methods["backtrace"] = &object.BuiltinMethod{Name: "backtrace", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if inst, ok := recv.(*object.Instance); ok {
			if v, ok := inst.Ivars["@__backtrace__"]; ok {
				return v, nil
			}
		}
		return object.NewArray(), nil
	}}
	root.Methods["full_message"] = root.Methods["message"]
	root.Methods["cause"] = &object.BuiltinMethod{Name: "cause", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if inst, ok := recv.(*object.Instance); ok {
			if v, ok := inst.Ivars["@__cause__"]; ok {
				return v, nil
			}
		}
		return object.NIL, nil
	}}
	env.SetGlobal("Exception", root)

	for _, def := range builtinExceptionTree {
		super, _ := env.Get(def.super)
		c := object.NewClass(def.name, super.(*object.Class))
		env.SetGlobal(def.name, c)
	}
	// SystemExit#status reads @status -- the integer exit code set
	// by Kernel#exit. Also success? returns true iff code == 0.
	if se, ok := env.Get("SystemExit"); ok {
		if c, ok := se.(*object.Class); ok {
			c.AddMethod("status", &object.BuiltinMethod{Name: "status", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
				if inst, ok := recv.(*object.Instance); ok {
					if v, ok := inst.Ivars["@status"]; ok {
						return v, nil
					}
				}
				return object.NewInteger(0), nil
			}})
			c.AddMethod("success?", &object.BuiltinMethod{Name: "success?", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
				if inst, ok := recv.(*object.Instance); ok {
					if i, ok := inst.Ivars["@status"].(*object.Integer); ok {
						return object.BooleanOf(i.Value == 0), nil
					}
				}
				return object.TRUE, nil
			}})
		}
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
	{"RegexpError", "StandardError"},
	{"IOError", "StandardError"},
	{"EOFError", "IOError"},
	{"SystemCallError", "StandardError"},
	{"Errno::ENOENT", "SystemCallError"},
	{"Errno::EACCES", "SystemCallError"},
	{"Errno::EEXIST", "SystemCallError"},
	{"Errno::EISDIR", "SystemCallError"},
	{"Errno::ENOTDIR", "SystemCallError"},
	{"ThreadError", "StandardError"},
	{"NoMemoryError", "Exception"},
	{"SignalException", "Exception"},
	{"SystemExit", "Exception"},
	{"SystemStackError", "Exception"},
	{"Interrupt", "SignalException"},
	{"ScriptError", "Exception"},
	{"SyntaxError", "ScriptError"},
}

// kernelRaise implements `raise`, in its three argument shapes.
func kernelRaise(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	bootstrapExceptionHierarchy(env)
	exc, err := buildRaisedException(env, args)
	if err != nil {
		return nil, err
	}
	// MRI: a raise inside a rescue body chains the currently-being-
	// handled exception as @__cause__ on the new one (unless the new
	// one already has a cause from an explicit `raise X, msg, cause`).
	if _, hasCause := exc.Ivars["@__cause__"]; !hasCause {
		for cur := env; cur != nil; cur = cur.Outer() {
			if cur.CurrentException != nil && cur.CurrentException != exc {
				if inst, ok := cur.CurrentException.(*object.Instance); ok && inst != exc {
					exc.Ivars["@__cause__"] = inst
				}
				break
			}
		}
	}
	attachBacktrace(env, exc)
	return nil, &raiseSignal{Exception: exc}
}

// attachBacktrace walks env's method-frame chain and stores a synthetic
// backtrace as @__backtrace__ on the exception instance. Each frame
// contributes one entry shaped like MRI's "<file>:in `<method>'". We
// don't track call-site line numbers yet, so the line is omitted; the
// shape is enough for callers that test inclusion of the method name.
func attachBacktrace(env *object.Environment, exc *object.Instance) {
	if exc == nil {
		return
	}
	if _, already := exc.Ivars["@__backtrace__"]; already {
		return
	}
	// Prefer the explicit call stack maintained by invokeMethodOn --
	// reversed so the innermost frame is first (MRI's backtrace
	// ordering). Falls back to walking env.Outer chain when the call
	// stack is empty.
	var frames []object.RubyObject
	if stk := env.CallStack(); len(stk) > 0 {
		for i := len(stk) - 1; i >= 0; i-- {
			frames = append(frames, object.NewString(stk[i]))
		}
	} else {
		for cur := env; cur != nil; cur = cur.Outer() {
			if !cur.MethodFrame || cur.CurrentMethodName == "" {
				continue
			}
			file := cur.CurrentFile()
			if file == "" {
				file = "(unknown)"
			}
			frames = append(frames, object.NewString(file+":in `"+cur.CurrentMethodName+"'"))
		}
	}
	exc.Ivars["@__backtrace__"] = object.NewArray(frames...)
}

func buildRaisedException(env *object.Environment, args []object.RubyObject) (*object.Instance, error) {
	runtimeErr, _ := env.Get("RuntimeError")
	runtimeClass := runtimeErr.(*object.Class)

	switch len(args) {
	case 0:
		// Bare `raise` inside a rescue body re-raises the current
		// exception. Walk the env for the nearest CurrentException;
		// outside a rescue, fall back to a blank RuntimeError per MRI.
		for cur := env; cur != nil; cur = cur.Outer() {
			if cur.CurrentException != nil {
				if inst, ok := cur.CurrentException.(*object.Instance); ok {
					return inst, nil
				}
			}
		}
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
		// MRI: raise(cls, arg) -> cls.new(arg). String args are the
		// common case; Exception instances flow through too
		// (UnexpectedError takes a wrapped exception).
		if msg, ok := stringText(env, args[1]); ok {
			return newExceptionInstance(cls, msg), nil
		}
		if inst, ok := args[1].(*object.Instance); ok {
			// Use the message string of the wrapped exception when
			// available; carry the original through as the cause.
			wrapped := ""
			if m, ok := inst.Ivars["@message"].(*object.String); ok {
				wrapped = string(m.Buf)
			} else {
				wrapped = env.Inspect(inst)
			}
			out := newExceptionInstance(cls, wrapped)
			out.Ivars["@__cause__"] = inst
			return out, nil
		}
		// Fall back to inspect form for any other value type.
		return newExceptionInstance(cls, env.Inspect(args[1])), nil
	}
	return nil, errorf("evaluator: ArgumentError: wrong number of arguments to raise (given %d)", len(args))
}

func newExceptionInstance(cls *object.Class, msg string) *object.Instance {
	inst := object.NewInstance(cls)
	inst.Ivars["@message"] = object.NewString(msg)
	// If the class chain has a user-defined initialize (not the
	// builtin Exception#initialize marker), call it so user state
	// like @targets gets set. Rake's RuleRecursionOverflowError
	// initialises @targets = [] in its own initialize.
	if m, found := cls.LookupMethod("initialize"); found {
		if um, ok := m.(*object.UserMethod); ok {
			if _, isMarker := um.Body.(exceptionInitMarker); !isMarker {
				args := []object.RubyObject{object.NewString(msg)}
				_, _ = invokeUserInitOn(inst, um, args)
			}
		}
	}
	return inst
}

// invokeUserInitOn calls a user-defined initialize on inst with args,
// going through the regular method-call path so block-bound parameters
// and rescue/ensure all behave as in normal call sites. Used from
// newExceptionInstance to wire user state on exceptions.
func invokeUserInitOn(inst *object.Instance, um *object.UserMethod, args []object.RubyObject) (object.RubyObject, error) {
	env := exceptionInitEnv(inst)
	return invokeMethodOn(env, inst, um, args, nil)
}

// exceptionInitEnv returns a small env rooted at inst's class so
// invokeMethodOn has a self-bound context.
func exceptionInitEnv(inst *object.Instance) *object.Environment {
	// Reuse the package-level "kernelSingleton" anchor env. NIL
	// suffices because invokeMethodOn builds its own frame env on top.
	return object.NewEnclosedEnvironment(nil)
}

// evalExceptionHandlingBlock implements begin/rescue/else/ensure. The
// match strategy: a rescue with no ExceptionClasses catches any
// StandardError descendant; otherwise the raised exception's class
// must descend from one of the listed classes.
func evalExceptionHandlingBlock(env *object.Environment, n *ast.ExceptionHandlingBlock) (object.RubyObject, error) {
	bootstrapExceptionHierarchy(env)

	var result object.RubyObject
	var err error
	for {
		result, err = evalBlockStatement(env, n.TryBody)

		if err == nil {
			if n.ElseBody != nil {
				result, err = evalBlockStatement(env, n.ElseBody)
			}
		}

		retried := false
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
				prevExc := env.CurrentException
				env.CurrentException = rs.Exception
				result, err = evalBlockStatement(env, r.Body)
				env.CurrentException = prevExc
				handled = true
				if _, isRetry := err.(*retrySignal); isRetry {
					retried = true
					err = nil
				}
				break
			}
			if !handled {
				err = rs
			}
		}
		if !retried {
			break
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
	exc := newExceptionInstance(c, msg)
	attachBacktrace(env, exc)
	return nil, &raiseSignal{Exception: exc}
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
		name := c.Value
		if strings.HasPrefix(name, "*") {
			// Splat-rescue: eval the bare name as an expression. The
			// stored value should be an Array of exception classes
			// (or a single class) -- iterate and match against each.
			// minitest's assert_raises does `rescue *exp` where exp
			// is the expected-classes array.
			obj, err := evalIdentifier(env, &ast.Identifier{Value: strings.TrimPrefix(name, "*")})
			if err != nil {
				continue
			}
			classes := []object.RubyObject{obj}
			if arr, ok := obj.(*object.Array); ok {
				classes = arr.Elements
			}
			for _, oc := range classes {
				cls, ok := oc.(*object.Class)
				if !ok {
					continue
				}
				if exc.C.IsAncestor(cls) {
					return true, nil
				}
			}
			continue
		}
		cls := resolveRescueClass(env, name)
		if cls == nil {
			// Couldn't resolve; skip rather than aborting the rescue
			// chain (matches MRI semantics: an unknown class in a
			// rescue list is a NameError, but we suppress it so the
			// other clauses still get a chance).
			continue
		}
		if exc.C.IsAncestor(cls) {
			return true, nil
		}
	}
	return false, nil
}

// resolveRescueClass resolves a rescue clause's class reference. The
// parser stores it as an Identifier whose Value is the source text
// (joined "::" for scoped names). Try the joined-name as a flat
// constant first (so built-in shapes like Errno::ENOENT registered
// via env.SetGlobal still resolve), then fall back to splitting and
// walking the lexical Constants chain.
func resolveRescueClass(env *object.Environment, name string) *object.Class {
	if v, ok := env.Get(name); ok {
		if c, ok := v.(*object.Class); ok {
			return c
		}
	}
	if !strings.Contains(name, "::") {
		// Walk the lexical chain too (an exception class declared
		// inside the current module is reachable bare).
		for cur := env; cur != nil; cur = cur.Outer() {
			if cls := cur.CurrentClass; cls != nil {
				if c, ok := cls.Constants[name].(*object.Class); ok {
					return c
				}
			}
		}
		return nil
	}
	parts := strings.Split(name, "::")
	var node *object.Class
	// Outermost: lexical chain then global.
	for cur := env; cur != nil; cur = cur.Outer() {
		if cls := cur.CurrentClass; cls != nil {
			if c, ok := cls.Constants[parts[0]].(*object.Class); ok {
				node = c
				break
			}
		}
	}
	if node == nil {
		if v, ok := env.Get(parts[0]); ok {
			if c, ok := v.(*object.Class); ok {
				node = c
			}
		}
	}
	if node == nil {
		return nil
	}
	for _, p := range parts[1:] {
		inner, ok := node.Constants[p].(*object.Class)
		if !ok {
			return nil
		}
		node = inner
	}
	return node
}
