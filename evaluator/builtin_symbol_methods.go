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
	add("name", toS)
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
	add("match?", func(env *object.Environment, r *object.Symbol, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 1 {
			return object.FALSE, nil
		}
		re, ok := args[0].(*object.Regex)
		if !ok {
			return object.FALSE, nil
		}
		return object.BooleanOf(re.RE.MatchString(env.Symbols().Name(r.ID))), nil
	})
	add("=~", func(env *object.Environment, r *object.Symbol, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 1 {
			return object.NIL, nil
		}
		return regexMatch(env, object.NewString(env.Symbols().Name(r.ID)), args[0]), nil
	})
	add("empty?", func(env *object.Environment, r *object.Symbol, args []object.RubyObject) (object.RubyObject, error) {
		return object.BooleanOf(env.Symbols().Name(r.ID) == ""), nil
	})
	add("upcase", func(env *object.Environment, r *object.Symbol, args []object.RubyObject) (object.RubyObject, error) {
		return env.Symbols().Intern(strings.ToUpper(env.Symbols().Name(r.ID))), nil
	})
	add("downcase", func(env *object.Environment, r *object.Symbol, args []object.RubyObject) (object.RubyObject, error) {
		return env.Symbols().Intern(strings.ToLower(env.Symbols().Name(r.ID))), nil
	})
}
