package object

import (
	"strings"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/trace"
)

type inspectable interface {
	Inspect() string
}

type hashable interface {
	HashKey() HashKey
}

// RubyObject represents an object in Ruby
type RubyObject interface {
	inspectable
	hashable
	Class() RubyClass
}

// RubyClass represents a class in Ruby
type RubyClass interface {
	inspectable
	hashable
	GetMethod(name string) (RubyMethod, bool)
	Methods() MethodSet
	New(args ...RubyObject) (RubyObject, error)
	Name() string
}

// RubyClassObject represents a class object in Ruby
type RubyClassObject interface {
	RubyObject
	RubyClass
}

// ReturnValue represents a wrapper object for a return statement. It is no
// real Ruby object and only used within the interpreter evaluation
type ReturnValue struct {
	Value RubyObject
}

func (rv *ReturnValue) Inspect() string  { return rv.Value.Inspect() }
func (rv *ReturnValue) Class() RubyClass { return rv.Value.Class() }
func (rv *ReturnValue) HashKey() HashKey { return rv.Value.HashKey() }

var (
	_ RubyObject = &ReturnValue{}
)

// BreakValue represents a wrapper object for a break statement. It is no
// real Ruby object and only used within the interpreter evaluation
type BreakValue struct {
	Value RubyObject
}

func (bv *BreakValue) Inspect() string  { return bv.Value.Inspect() }
func (bv *BreakValue) Class() RubyClass { return bv.Value.Class() }
func (bv *BreakValue) HashKey() HashKey { return bv.Value.HashKey() }

var (
	_ RubyObject = &BreakValue{}
)

// FunctionParameters represents a list of function parameters.
type functionParameters []*FunctionParameter

func (f functionParameters) defaultParamCount() int {
	count := 0
	for _, p := range f {
		if p.Default != nil {
			count++
		}
	}
	return count
}

func (f functionParameters) separateDefaultParams() ([]*FunctionParameter, []*FunctionParameter) {
	mandatory, defaults := make([]*FunctionParameter, 0), make([]*FunctionParameter, 0)
	for _, p := range f {
		if p.Default != nil {
			defaults = append(defaults, p)
		} else {
			mandatory = append(mandatory, p)
		}
	}
	return mandatory, defaults
}

// FunctionParameter represents a parameter within a function
type FunctionParameter struct {
	Name    string
	Default RubyObject
	Splat   bool
}

func (f *FunctionParameter) String() string {
	var out strings.Builder
	if f.Splat {
		out.WriteString("*")
	}
	out.WriteString(f.Name)
	if f.Default != nil {
		out.WriteString(" = ")
		out.WriteString(f.Default.Inspect())
	}
	return out.String()
}

// A Function represents a user defined function. It is no real Ruby object.
type Function struct {
	Name       string
	Parameters []*FunctionParameter
	Body       *ast.BlockStatement
	Env        Environment
}

// String returns the function literal
func (f *Function) String() string {
	var out strings.Builder
	out.WriteString("{")
	if len(f.Parameters) != 0 {
		args := []string{}
		for _, a := range f.Parameters {
			args = append(args, a.String())
		}
		out.WriteString("|")
		out.WriteString(strings.Join(args, ", "))
		out.WriteString("|")
	}
	out.WriteString("")
	out.WriteString(f.Body.String())
	out.WriteString("}")
	return out.String()
}

// Call implements the RubyMethod interface. It evaluates f.Body and returns its result
func (f *Function) Call(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace("Function.Call"))
		tracer.Message(f.Name)
		tracer.Message(f.String())
	}
	// TODO: Handle tail splats
	if len(f.Parameters) == 1 && f.Parameters[0].Splat {
		// Only one splat parameter.
		args_arr := NewArray(args...)
		extendedEnv := NewEnclosedEnvironment(f.Env)
		extendedEnv.Set(f.Parameters[0].Name, args_arr)
		evaluated, err := context.Eval(f.Body, extendedEnv)
		if err != nil {
			return nil, err
		}
		return f.unwrapReturnValue(evaluated), nil

	} else {
		// normal evaluation
		defaultParams := functionParameters(f.Parameters).defaultParamCount()
		if len(args) < len(f.Parameters)-defaultParams || len(args) > len(f.Parameters) {
			return nil, NewWrongNumberOfArgumentsError(len(f.Parameters), len(args))
		}
		params, err := f.populateParameters(args)
		if err != nil {
			return nil, err
		}
		extendedEnv := NewEnclosedEnvironment(f.Env)
		for k, v := range params {
			extendedEnv.Set(k, v)
		}
		evaluated, err := context.Eval(f.Body, extendedEnv)
		if err != nil {
			return nil, err
		}
		return f.unwrapReturnValue(evaluated), nil
	}
}

func (f *Function) populateParameters(args []RubyObject) (map[string]RubyObject, error) {
	if len(args) > len(f.Parameters) {
		return nil, NewWrongNumberOfArgumentsError(len(f.Parameters), len(args))
	}
	params := make(map[string]RubyObject)

	mandatory, defaults := functionParameters(f.Parameters).separateDefaultParams()

	if len(args) < len(mandatory)-len(defaults) || len(args) > len(f.Parameters) {
		return nil, NewWrongNumberOfArgumentsError(len(f.Parameters), len(args))
	}

	if len(args) == len(f.Parameters) {
		for paramIdx, param := range f.Parameters {
			params[param.Name] = args[paramIdx]
		}
		return params, nil
	}

	parameters := append(mandatory, defaults...)

	for paramIdx, param := range parameters {
		if paramIdx >= len(args) {
			params[param.Name] = param.Default
			continue
		}
		params[param.Name] = args[paramIdx]
	}
	return params, nil
}

func (f *Function) unwrapReturnValue(obj RubyObject) RubyObject {
	if returnValue, ok := obj.(*ReturnValue); ok {
		return returnValue.Value
	}
	return obj
}
