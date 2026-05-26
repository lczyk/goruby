package evaluator

import (
	"unicode/utf8"

	"github.com/lczyk/goruby/object"
)

// String method names registered on StringClass. Each adapter routes
// into the per-name body in callStringMethod (string.go), which is the
// String-side analogue of callArrayMethod / callHashMethod.
var stringMethodNames = []string{
	"length", "size", "upcase", "downcase", "capitalize", "swapcase",
	"strip", "lstrip", "rstrip", "reverse", "include?", "to_s", "empty?",
	"chars", "lines",
	"split", "chomp", "start_with?", "end_with?", "replace", "match?",
	"match", "scan", "sub", "sub!", "gsub", "ord", "to_sym", "tr", "tr_s", "count",
	"bytes", "bytesize", "ljust", "rjust", "center", "succ", "next",
	"squeeze", "prepend", "delete",
	"to_i", "to_f", "unpack",
	"dup", "clone", "force_encoding", "scrub", "encoding", "encode",
	"gsub!", "strip!", "lstrip!", "rstrip!", "upcase!", "downcase!",
	"slice", "[]",
}

func init() {
	c := object.StringClass
	for _, name := range stringMethodNames {
		n := name
		c.AddMethod(n, &object.BuiltinMethod{
			Name: n,
			Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
				v, _, err := callStringMethod(env, recv, n, args)
				return v, err
			},
		})
	}
	// Operator methods. Wrap stringInfix so `s.send(:+, t)` and
	// friends resolve the same way as `s + t`.
	for _, op := range []string{"+", "*", "%", "<", "<=", ">", ">=", "<=>"} {
		n := op
		c.AddMethod(n, &object.BuiltinMethod{
			Name: n,
			Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
				if len(args) != 1 {
					return nil, errorf("evaluator: String#%s expects 1 arg, got %d", n, len(args))
				}
				v, handled, err := stringInfix(env, n, recv, args[0])
				if err != nil {
					return nil, err
				}
				if !handled {
					return nil, errorf("evaluator: String#%s: unsupported operand %T", n, args[0])
				}
				return v, nil
			},
		})
	}
	// valid_encoding? -- check UTF-8 validity of the buffer. MRI's
	// default encoding is UTF-8 in goruby's universe.
	c.AddMethod("valid_encoding?", &object.BuiltinMethod{Name: "valid_encoding?", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		s, ok := stringText(env, recv)
		if !ok {
			return object.FALSE, nil
		}
		return object.BooleanOf(utf8.ValidString(s)), nil
	}})
	// scan with a block: iterate matches and yield each to the block;
	// returns self. No block: identical to the non-block scan handler.
	c.AddMethod("scan", &object.BuiltinMethod{Name: "scan", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if block == nil {
			v, _, err := callStringMethod(env, recv, "scan", args)
			return v, err
		}
		s, _ := stringText(env, recv)
		if len(args) != 1 {
			return nil, errorf("evaluator: String#scan expects 1 arg")
		}
		re, ok := args[0].(*object.Regex)
		if !ok {
			return nil, errorf("evaluator: String#scan with block: needs Regexp")
		}
		for _, m := range re.RE.FindAllStringSubmatch(s, -1) {
			var arg object.RubyObject
			if len(m) == 1 {
				arg = object.NewString(m[0])
			} else {
				inner := make([]object.RubyObject, len(m)-1)
				for i, g := range m[1:] {
					inner[i] = object.NewString(g)
				}
				arg = object.NewArray(inner...)
			}
			if _, err := invokeBlockValue(env, block, []object.RubyObject{arg}); err != nil {
				return nil, err
			}
		}
		return recv, nil
	}})
	// =~ -- matches a regexp, returns the byte index or nil. Goes
	// through the same regexMatch helper as the infix =~ so $~/$1
	// globals get populated identically.
	c.AddMethod("=~", &object.BuiltinMethod{Name: "=~", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: String#=~ expects 1 arg, got %d", len(args))
		}
		return regexMatch(env, recv, args[0]), nil
	}})

	// == and << aren't handled by stringInfix -- implement directly.
	c.AddMethod("==", &object.BuiltinMethod{Name: "==", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		l, _ := stringText(env, recv)
		r, ok := stringText(env, args[0])
		if !ok {
			return object.FALSE, nil
		}
		return object.BooleanOf(l == r), nil
	}})
	c.AddMethod("<<", &object.BuiltinMethod{Name: "<<", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		s, ok := recv.(*object.String)
		if !ok {
			return nil, errorf("evaluator: String#<<: receiver must be mutable String, got %T", recv)
		}
		add, ok := stringText(env, args[0])
		if !ok {
			return nil, errorf("evaluator: String#<<: operand must be a String, got %T", args[0])
		}
		s.Buf = append(s.Buf, []byte(add)...)
		return s, nil
	}})
}
