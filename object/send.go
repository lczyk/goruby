package object

import (
	"hash/fnv"

	"github.com/lczyk/goruby/trace"
)

// Send sends message method with args to context and returns its result
func Send(context CallContext, method string, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
		tracer.Message(method)
	}
	receiver := context.Receiver()
	class := receiver.Class()

	// search for the method in the ancestry tree
	for class != nil {
		fn, ok := class.GetMethod(method)
		if !ok {
			if class == bottomClass {
				// no method and we are at the top of the ancestry tree
				break
			}
			class = bottomClass
			continue
		}

		return fn.Call(context, tracer, args...)
	}

	// fmt.Printf("receiver: %v(%T)\n", receiver, receiver)
	// fmt.Printf("method: %v(%T)\n", method, method)
	return nil, NewNoMethodError(receiver, method)
}

func newEigenclass(wrappedClass RubyClass) *eigenclass {
	return &eigenclass{
		methods:      NewMethodSet(nil),
		wrappedClass: wrappedClass,
	}
}

type eigenclass struct {
	methods      SettableMethodSet
	wrappedClass RubyClass
}

func (e *eigenclass) Inspect() string {
	if e.wrappedClass == nil_class {
		return "(eigenclass of nil)"
	}
	if e.wrappedClass == nil {
		return "(singleton class)"
	}
	return e.wrappedClass.(RubyObject).Inspect()
}

func (e *eigenclass) Class() RubyClass {
	return e.wrappedClass
}
func (e *eigenclass) Methods() MethodSet { return e.methods }
func (e *eigenclass) GetMethod(name string) (RubyMethod, bool) {
	if method, ok := e.methods.Get(name); ok {
		return method, true
	}
	return nil, false
}

func (e *eigenclass) SuperClass() RubyClass {
	if e.wrappedClass != nil {
		return e.wrappedClass
	}
	return bottomClass
}
func (e *eigenclass) New(args ...RubyObject) (RubyObject, error) {
	return e.wrappedClass.New(args...)
}
func (e *eigenclass) Name() string { return e.wrappedClass.Name() }
func (e *eigenclass) addMethod(name string, method RubyMethod) {
	e.methods.Set(name, method)
}
func (e *eigenclass) HashKey() HashKey {
	if e.wrappedClass != nil {
		h := fnv.New64a()
		h.Write([]byte(e.wrappedClass.Name()))
		return HashKey(h.Sum64())
	}
	// NOTE: temp fix.
	return HashKey(1)
}

var (
	_ RubyObject = &eigenclass{}
	_ RubyClass  = &eigenclass{}
)

// extendedObject is a wrapper object for an object extended by methods.
type extendedObject struct {
	RubyObject
	eigenclass *eigenclass
}

func newExtendedObject(object RubyObject) *extendedObject {
	object_class := object.Class()
	if object_class == nil {
		panic("object_class is nil")
	}
	if object_class == nil_class {
		panic("object_class is nil_class")
	}

	return &extendedObject{
		RubyObject: object,
		eigenclass: newEigenclass(object_class),
	}
}

func (e *extendedObject) Class() RubyClass { return e.eigenclass }
func (e *extendedObject) Inspect() string  { return e.RubyObject.Inspect() }

// func (e *extendedObject) String() string { return "hello" }
func (e *extendedObject) addMethod(name string, method RubyMethod) {
	e.eigenclass.addMethod(name, method)
}

// func (e *extendedObject) GetMethod(
func (e *extendedObject) GetMethod(name string) (RubyMethod, bool) {
	if method, ok := e.RubyObject.Class().GetMethod(name); ok {
		return method, true
	}
	return e.eigenclass.GetMethod(name)
}

var (
	_ RubyObject = &extendedObject{}
	_ extendable = &extendedObject{}
)

type extendable interface {
	addMethod(name string, method RubyMethod)
}

type extendableRubyObject interface {
	RubyObject
	extendable
}

// AddMethod adds a method to a given object. It returns the object with the modified method set
func AddMethod(context RubyObject, methodName string, method *Function) (RubyObject, bool) {
	extended, is_extendable := context.(extendableRubyObject)
	if !is_extendable {
		extended = newExtendedObject(context)
	}
	extended.addMethod(methodName, method)
	return extended, !is_extendable
}
