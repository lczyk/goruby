package object

import "github.com/lczyk/goruby/trace"

type RubyMethod interface {
	Call(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error)
}

func withArity(arity int, fn RubyMethod) RubyMethod {
	return &method{
		fn: func(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
			if tracer != nil {
				defer tracer.Un(tracer.Trace("withArity"))
			}
			if len(args) != arity {
				return nil, NewWrongNumberOfArgumentsError(arity, len(args))
			}
			return fn.Call(context, tracer, args...)
		},
	}
}

func newMethod(fn func(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error)) RubyMethod {
	return &method{fn: fn}
}

type method struct {
	fn func(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error)
}

func (m *method) Call(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace("method.Call"))
	}
	return m.fn(context, tracer, args...)
}

// MethodSet represents a set of methods
type MethodSet interface {
	// Get returns the method found for name. The boolean will return true if
	// a method was found, false otherwise
	Get(name string) (RubyMethod, bool)
	// Names returns the names of all methods in the set
	Names() []string
}

// SettableMethodSet represents a MethodSet which can be mutated by setting
// methods on it.
type SettableMethodSet interface {
	MethodSet
	// Set will set method to key name. If there was a method prior defined
	// under name it will be overridden.
	Set(name string, method RubyMethod)
}

// NewMethodSet returns a new method set populated with the given methods
func NewMethodSet(methods map[string]RubyMethod) SettableMethodSet {
	if methods == nil {
		methods = make(map[string]RubyMethod)
	}
	return &methodSet{methods: methods}
}

type methodSet struct {
	methods map[string]RubyMethod
}

func (m *methodSet) Names() []string {
	methods := make([]string, 0, len(m.methods))
	for k := range m.methods {
		methods = append(methods, k)
	}
	return methods
}

func (m *methodSet) Get(name string) (RubyMethod, bool) {
	method, ok := m.methods[name]
	return method, ok
}

func (m *methodSet) Set(name string, method RubyMethod) {
	m.methods[name] = method
}
