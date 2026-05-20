package evaluator

import (
	"strings"

	"github.com/lczyk/goruby/object"
)

func init() {
	c := object.SymbolClass
	add := func(name string, fn func(env *object.Environment, r *object.Symbol, args []object.RubyObject) (object.RubyObject, error)) {
		c.AddMethod(name, &object.BuiltinMethod{
			Name: name,
			Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
				return fn(env, recv.(*object.Symbol), args)
			},
		})
	}

	toS := func(env *object.Environment, r *object.Symbol, args []object.RubyObject) (object.RubyObject, error) {
		return object.NewString(env.Symbols().Name(r.ID)), nil
	}
	add("to_s", toS)
	add("id2name", toS)
	add("to_sym", func(env *object.Environment, r *object.Symbol, args []object.RubyObject) (object.RubyObject, error) {
		return r, nil
	})
	add("to_proc", func(env *object.Environment, r *object.Symbol, args []object.RubyObject) (object.RubyObject, error) {
		return procFromSymbol(env.Symbols().Name(r.ID)), nil
	})
	sz := func(env *object.Environment, r *object.Symbol, args []object.RubyObject) (object.RubyObject, error) {
		return object.NewInteger(int64(len(env.Symbols().Name(r.ID)))), nil
	}
	add("length", sz)
	add("size", sz)
	add("upcase", func(env *object.Environment, r *object.Symbol, args []object.RubyObject) (object.RubyObject, error) {
		return env.Symbols().Intern(strings.ToUpper(env.Symbols().Name(r.ID))), nil
	})
	add("downcase", func(env *object.Environment, r *object.Symbol, args []object.RubyObject) (object.RubyObject, error) {
		return env.Symbols().Intern(strings.ToLower(env.Symbols().Name(r.ID))), nil
	})
}
