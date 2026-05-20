package evaluator

import (
	"github.com/lczyk/goruby/object"
)

// dataInitMarker is the synthesised initialize body installed by
// Data.define. Holds the field names; dispatchAttrMarker reads
// callEnv.CurrentKwargs and writes ivars by those names.
type dataInitMarker []string

// dataDefine implements ruby 3.2's `Data.define(:x, :y, ...)`: returns
// a fresh value-object class with attr_readers for each field and a
// kwargs-only initialize.
func dataDefine(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	fields := make([]string, 0, len(args))
	for _, a := range args {
		s, ok := symbolOrString(env, a)
		if !ok {
			return nil, errorf("evaluator: Data.define expected Symbol or String, got %T", a)
		}
		fields = append(fields, s)
	}
	c := object.NewClass("Data", nil)
	for _, f := range fields {
		c.Methods[f] = makeAttrReader(f)
	}
	c.Methods["initialize"] = &object.UserMethod{
		Name: "initialize",
		Body: dataInitMarker(fields),
	}
	return c, nil
}

// bootstrapDataClass installs the predefined `Data` class so the
// `Data.define(...)` constant lookup resolves.
func bootstrapDataClass(env *object.Environment) *object.Class {
	if existing, ok := env.Get("Data"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	c := object.NewClass("Data", nil)
	env.SetGlobal("Data", c)
	// Struct is similar enough that we register it here too; the
	// difference (Struct allows positional .new and is mutable) is
	// handled in classNew + structDefine.
	if _, ok := env.Get("Struct"); !ok {
		s := object.NewClass("Struct", nil)
		env.SetGlobal("Struct", s)
	}
	return c
}

// structDefine implements `Struct.new(:x, :y)`: returns a fresh class
// with attr_accessors for each field and a positional initialize.
func structDefine(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	fields := make([]string, 0, len(args))
	for _, a := range args {
		s, ok := symbolOrString(env, a)
		if !ok {
			return nil, errorf("evaluator: Struct.new expected Symbol/String, got %T", a)
		}
		fields = append(fields, s)
	}
	c := object.NewClass("StructClass", nil)
	for _, f := range fields {
		c.Methods[f] = makeAttrReader(f)
		c.Methods[f+"="] = makeAttrWriter(f)
	}
	c.Methods["initialize"] = &object.UserMethod{
		Name: "initialize",
		Body: structInitMarker(fields),
	}
	c.Methods["to_a"] = &object.UserMethod{Name: "to_a", Body: structToAMarker(fields)}
	c.Methods["members"] = &object.UserMethod{Name: "members", Body: structMembersMarker(fields)}
	return c, nil
}

type structInitMarker []string
type structToAMarker []string
type structMembersMarker []string
