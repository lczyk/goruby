package evaluator

import (
	"math/big"
	"path/filepath"
	"strings"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/object"
)

// Eval walks node and returns its runtime value. Errors are evaluator
// errors (programmer / unimplemented), distinct from ruby exceptions.
func Eval(node ast.Node, env *object.Environment) (object.RubyObject, error) {
	switch n := node.(type) {

	case *ast.Program:
		return evalProgram(env, n)

	case *ast.ExpressionStatement:
		return Eval(n.Expression, env)

	// Literals -------------------------------------------------------

	case *ast.IntegerLiteral:
		return object.NewInteger(n.Value), nil

	case *ast.FloatLiteral:
		return object.NewFloat(n.Value), nil

	case *ast.Boolean:
		return object.BooleanOf(n.Value), nil

	case *ast.Nil:
		return object.NIL, nil

	case *ast.StringLiteral:
		return evalStringLiteral(env, n)

	case *ast.SymbolLiteral:
		return evalSymbolLiteral(env, n)

	case *ast.RegexLiteral:
		return evalRegexLiteral(env, n)

	case *ast.ArrayLiteral:
		return evalArrayLiteral(env, n)

	case *ast.HashLiteral:
		return evalHashLiteral(env, n)

	// Variables ------------------------------------------------------

	case *ast.Identifier:
		return evalIdentifier(env, n)

	case *ast.Global:
		return evalGlobal(env, n)

	case *ast.Assignment:
		return evalAssignment(env, n)

	case ast.ExpressionList:
		return evalExpressionList(env, n)

	// Compound -------------------------------------------------------

	case *ast.PrefixExpression:
		return evalPrefix(env, n)

	case *ast.InfixExpression:
		return evalInfix(env, n)

	case *ast.IndexExpression:
		return evalIndex(env, n)

	case *ast.ContextCallExpression:
		return evalContextCall(env, n)

	case *ast.ParenExpression:
		return evalParen(env, n)

	// Control flow ---------------------------------------------------

	case *ast.ConditionalExpression:
		return evalConditional(env, n)

	case *ast.LoopExpression:
		return evalLoop(env, n)

	case *ast.CaseExpression:
		return evalCase(env, n)

	case *ast.BlockStatement:
		return evalBlockStatement(env, n)

	// Methods --------------------------------------------------------

	case *ast.FunctionLiteral:
		return evalFunctionLiteral(env, n)

	case *ast.ReturnStatement:
		return evalReturn(env, n)

	case *ast.JumpExpression:
		return evalJump(env, n)

	case *ast.YieldExpression:
		return evalYield(env, n)

	case *ast.DefinedExpression:
		return evalDefined(env, n)

	// Classes --------------------------------------------------------

	case *ast.ClassExpression:
		return evalClassExpression(env, n)

	case *ast.InstanceVariable:
		return evalInstanceVariable(env, n)

	case *ast.ClassVariable:
		return evalClassVariable(env, n)

	case *ast.Self:
		return evalSelf(env)

	case *ast.SuperExpression:
		return evalSuper(env, n)

	case *ast.ModuleExpression:
		return evalModuleExpression(env, n)

	case *ast.ScopedIdentifier:
		return evalScopedIdentifier(env, n)

	case *ast.ExceptionHandlingBlock:
		return evalExceptionHandlingBlock(env, n)

	case *ast.Keyword__FILE__:
		return object.NewString(n.Filename), nil

	case *ast.AliasExpression:
		return evalAlias(env, n)

	case *ast.SplatExpression:
		// Bare `*expr` used as a value (e.g. `a, b = *foo`) -- evaluate
		// the operand and let downstream unpacking (unpackMultiRHS,
		// call-arg splat handling, etc.) treat the result as an array
		// to spread. SplatExpression in positions that need explicit
		// splat handling (Array literal, call args, multi-assign LHS)
		// are intercepted before reaching this case.
		if n.Operator != "*" {
			return nil, errorf("evaluator: unsupported splat operator %q", n.Operator)
		}
		return Eval(n.Right, env)
	}

	return nil, errorf("evaluator: unhandled AST node type %T", node)
}

func evalParen(env *object.Environment, n *ast.ParenExpression) (object.RubyObject, error) {
	// Multi-statement form `(a; b; c)` returns the last value.
	if len(n.Stmts) > 0 {
		var result object.RubyObject = object.NIL
		for _, e := range n.Stmts {
			v, err := Eval(e, env)
			if err != nil {
				return nil, err
			}
			result = v
		}
		return result, nil
	}
	return Eval(n.Expr, env)
}

func evalProgram(env *object.Environment, p *ast.Program) (object.RubyObject, error) {
	bootstrapBuiltins(env)
	// Ruby introduces a local variable at parser-time as soon as it sees
	// an assignment to it, even on branches that never execute -- the
	// variable then reads as nil after that point. Mirror by pre-binding
	// every plain identifier assigned anywhere in the top-level scope
	// (skipping def/class/module/block scopes) to nil before evaluating.
	predeclareTopLevelLocals(env, p.Statements)
	// Stamp the active source-file path so Kernel#require_relative can
	// resolve siblings via dirname(currentFile). Only set when the
	// Program carries one (parser-built); restore prior on exit so
	// nested loads pop cleanly.
	if p.Filename != "" {
		abs := p.Filename
		if !filepath.IsAbs(abs) {
			if a, err := filepath.Abs(abs); err == nil {
				abs = a
			}
		}
		prev := env.SetCurrentFile(abs)
		defer func() { env.SetCurrentFile(prev) }()
	}
	var result object.RubyObject = object.NIL
	for _, stmt := range p.Statements {
		v, err := Eval(stmt, env)
		if err != nil {
			// Kernel#exit / Kernel#abort short-circuit the program
			// cleanly. Code 0 returns nil with no error so test
			// harnesses see a normal end; non-zero bubbles as a
			// regular error string so callers can act on it.
			if es, ok := err.(*exitSignal); ok {
				if es.Code == 0 {
					return object.NIL, nil
				}
				return nil, errorf("exit: %d", es.Code)
			}
			return nil, err
		}
		result = v
	}
	return result, nil
}

func evalStringLiteral(env *object.Environment, n *ast.StringLiteral) (object.RubyObject, error) {
	if len(n.Parts) > 0 {
		var b strings.Builder
		for _, p := range n.Parts {
			switch part := p.(type) {
			case *ast.StringContent:
				b.WriteString(decodeStringEscapes(part.Value, false))
			default:
				v, err := Eval(p, env)
				if err != nil {
					return nil, err
				}
				b.WriteString(toStringValue(env, v))
			}
		}
		return object.NewString(b.String()), nil
	}
	// Parser keeps escape sequences verbatim in Value (for source-faithful
	// roundtripping). The evaluator decodes them per the quote style.
	decoded := decodeStringEscapes(n.Value, n.Token.SingleQuoted)
	return object.NewString(decoded), nil
}

// toStringValue is the Kernel#to_s rendering used during string
// interpolation. Mirrors MRI: nil -> "", strings/symbols bare, others
// fall back to inspect.
func toStringValue(env *object.Environment, o object.RubyObject) string {
	switch v := o.(type) {
	case *object.Nil:
		return ""
	case *object.String:
		return string(v.Buf)
	case *object.FrozenString:
		return env.Strings().Get(v.ID)
	case *object.Symbol:
		return env.Symbols().Name(v.ID)
	}
	return env.Inspect(o)
}

// decodeStringEscapes interprets backslash escapes in raw source text.
// Double-quoted strings honour the common C-style escapes (\n, \t, \r,
// \\, \", \0, \a, \b, \e, \f, \v, \s); single-quoted strings only honour
// \\ and \'. Unknown escapes in double-quoted strings drop the
// backslash, matching MRI.
// hexDigit returns the decimal value of the hex digit at s[i],
// ok=true if it's a valid hex digit, false otherwise / out-of-bounds.
func hexDigit(s string, i int) (int, bool) {
	if i < 0 || i >= len(s) {
		return 0, false
	}
	c := s[i]
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10, true
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10, true
	}
	return 0, false
}

func decodeStringEscapes(s string, singleQuoted bool) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			b.WriteByte(c)
			continue
		}
		next := s[i+1]
		if singleQuoted {
			switch next {
			case '\\', '\'':
				b.WriteByte(next)
				i++
				continue
			}
			b.WriteByte(c)
			continue
		}
		switch next {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '\\':
			b.WriteByte('\\')
		case '"':
			b.WriteByte('"')
		case '\'':
			b.WriteByte('\'')
		case '0':
			b.WriteByte(0)
		case 'a':
			b.WriteByte(7)
		case 'b':
			b.WriteByte(8)
		case 'e':
			b.WriteByte(27)
		case 'f':
			b.WriteByte(12)
		case 'v':
			b.WriteByte(11)
		case 's':
			b.WriteByte(' ')
		case 'x':
			// \xHH -- hex byte escape. Consume one or two hex digits;
			// missing digits keep the literal \x in place (matches
			// mri's "stop on first non-hex" rule).
			hi, hiOK := hexDigit(s, i+2)
			if !hiOK {
				b.WriteByte('x')
				break
			}
			lo, loOK := hexDigit(s, i+3)
			if loOK {
				b.WriteByte(byte(hi*16 + lo))
				i += 2 // consume two more (after the `x`)
			} else {
				b.WriteByte(byte(hi))
				i++
			}
		default:
			b.WriteByte(next)
		}
		i++
	}
	return b.String()
}

func evalSymbolLiteral(env *object.Environment, n *ast.SymbolLiteral) (object.RubyObject, error) {
	// Parser stores the symbol's name as either *ast.Identifier (`:foo`)
	// or *ast.StringLiteral (`a:` hash label or `:"...#{}..."` form).
	// Interpolated string form evaluates each part and concatenates.
	if sl, ok := n.Value.(*ast.StringLiteral); ok && len(sl.Parts) > 0 {
		v, err := evalStringLiteral(env, sl)
		if err != nil {
			return nil, err
		}
		s, _ := stringText(env, v)
		return env.Symbols().Intern(s), nil
	}
	name, err := symbolName(n.Value)
	if err != nil {
		return nil, err
	}
	return env.Symbols().Intern(name), nil
}

func symbolName(expr ast.Expression) (string, error) {
	switch v := expr.(type) {
	case *ast.Identifier:
		return v.Value, nil
	case *ast.StringLiteral:
		if len(v.Parts) > 0 {
			return "", errorf("evaluator: interpolated symbol not yet supported")
		}
		return v.Value, nil
	case *ast.InstanceVariable:
		return "@" + v.Name.Value, nil
	case *ast.ClassVariable:
		return "@@" + v.Name.Value, nil
	case *ast.Global:
		return "$" + v.Value, nil
	}
	return "", errorf("evaluator: malformed SymbolLiteral.Value: %T", expr)
}

func evalArrayLiteral(env *object.Environment, n *ast.ArrayLiteral) (object.RubyObject, error) {
	elems := make([]object.RubyObject, 0, len(n.Elements))
	for _, e := range n.Elements {
		if sp, ok := e.(*ast.SplatExpression); ok && sp.Operator == "*" {
			v, err := Eval(sp.Right, env)
			if err != nil {
				return nil, err
			}
			if arr, ok := v.(*object.Array); ok {
				elems = append(elems, arr.Elements...)
				continue
			}
			elems = append(elems, v)
			continue
		}
		v, err := Eval(e, env)
		if err != nil {
			return nil, err
		}
		elems = append(elems, v)
	}
	return object.NewArray(elems...), nil
}

func evalHashLiteral(env *object.Environment, n *ast.HashLiteral) (object.RubyObject, error) {
	entries := make([]object.HashEntry, 0, n.Map.Len()+len(n.Splats))
	for _, kv := range n.Map.Entries() {
		k, err := Eval(kv.Key, env)
		if err != nil {
			return nil, err
		}
		v, err := Eval(kv.Value, env)
		if err != nil {
			return nil, err
		}
		entries = append(entries, object.HashEntry{Key: k, Value: v})
	}
	for _, sp := range n.Splats {
		v, err := Eval(sp, env)
		if err != nil {
			return nil, err
		}
		h, ok := v.(*object.Hash)
		if !ok {
			return nil, errorf("evaluator: hash splat (**) requires Hash, got %T", v)
		}
		for _, e := range h.Entries {
			// Merge semantics: a later occurrence of the same key
			// overwrites the earlier one.
			replaced := false
			for i := range entries {
				if rubyEqualDispatch(env, entries[i].Key, e.Key) {
					entries[i].Value = e.Value
					replaced = true
					break
				}
			}
			if !replaced {
				entries = append(entries, object.HashEntry{Key: e.Key, Value: e.Value})
			}
		}
	}
	return object.NewHash(entries...), nil
}

func evalPrefix(env *object.Environment, n *ast.PrefixExpression) (object.RubyObject, error) {
	right, err := Eval(n.Right, env)
	if err != nil {
		return nil, err
	}
	switch n.Operator {
	case "-":
		if i, ok := right.(*object.Integer); ok {
			return object.NewInteger(-i.Value), nil
		}
		if f, ok := right.(*object.Float); ok {
			return object.NewFloat(-f.Value), nil
		}
		if _, ok := right.(*object.Instance); ok {
			return callMethod(env, right, "-@", nil)
		}
	case "!", "not":
		return object.BooleanOf(!truthy(right)), nil
	case "+":
		if _, ok := right.(*object.Instance); ok {
			return callMethod(env, right, "+@", nil)
		}
		return right, nil
	case "~":
		if i, ok := right.(*object.Integer); ok {
			return object.NewInteger(^i.Value), nil
		}
	}
	return nil, errorf("evaluator: unsupported prefix %q on %T", n.Operator, right)
}

func evalInfix(env *object.Environment, n *ast.InfixExpression) (object.RubyObject, error) {
	// Short-circuit operators: evaluate the right side only when needed
	// and preserve ruby's "return the operand that decided the result"
	// semantics (not coerced to bool).
	switch n.Operator {
	case "&&", "and":
		left, err := Eval(n.Left, env)
		if err != nil {
			return nil, err
		}
		if !truthy(left) {
			return left, nil
		}
		return Eval(n.Right, env)
	case "||", "or":
		left, err := Eval(n.Left, env)
		if err != nil {
			return nil, err
		}
		if truthy(left) {
			return left, nil
		}
		return Eval(n.Right, env)
	}

	left, err := Eval(n.Left, env)
	if err != nil {
		return nil, err
	}
	right, err := Eval(n.Right, env)
	if err != nil {
		return nil, err
	}

	// User-defined operator methods on the LHS instance take precedence
	// over the built-in Object equality check below, mirroring MRI.
	if _, ok := left.(*object.Instance); ok {
		switch n.Operator {
		case "+", "-", "*", "/", "%", "**", "<", "<=", ">", ">=", "<=>", "==":
			return callMethod(env, left, n.Operator, []object.RubyObject{right})
		case "!=":
			eq, err := callMethod(env, left, "==", []object.RubyObject{right})
			if err != nil {
				return nil, err
			}
			return object.BooleanOf(!truthy(eq)), nil
		}
	}

	switch n.Operator {
	case "=~":
		return regexMatch(env, left, right), nil
	case "!~":
		v := regexMatch(env, left, right)
		if _, isNil := v.(*object.Nil); isNil {
			return object.TRUE, nil
		}
		return object.FALSE, nil
	case "==":
		return object.BooleanOf(rubyEqual(left, right)), nil
	case "!=":
		return object.BooleanOf(!rubyEqual(left, right)), nil
	case "..":
		return object.NewRange(left, right, false), nil
	case "...":
		return object.NewRange(left, right, true), nil
	case "<<":
		if arr, ok := left.(*object.Array); ok {
			arr.Elements = append(arr.Elements, right)
			return arr, nil
		}
		if s, ok := left.(*object.String); ok {
			if t, ok := stringText(env, right); ok {
				s.Buf = append(s.Buf, t...)
				return s, nil
			}
			if i, ok := right.(*object.Integer); ok {
				s.Buf = append(s.Buf, string(rune(i.Value))...)
				return s, nil
			}
		}
		// IO classes (STDOUT / STDERR) treat `<<` as write-and-return-self.
		// Dispatch through the class's write method when present.
		if c, ok := left.(*object.Class); ok {
			if m, found := c.LookupClassMethod("write"); found {
				if um, ok := m.(*object.UserMethod); ok {
					if _, err := callUserMethod(env, um, []object.RubyObject{right}); err != nil {
						return nil, err
					}
					return c, nil
				}
			}
		}
	case "+":
		if larr, ok := left.(*object.Array); ok {
			if rarr, ok := right.(*object.Array); ok {
				out := make([]object.RubyObject, 0, len(larr.Elements)+len(rarr.Elements))
				out = append(out, larr.Elements...)
				out = append(out, rarr.Elements...)
				return object.NewArray(out...), nil
			}
		}
		if lh, ok := left.(*object.Hash); ok {
			if rh, ok := right.(*object.Hash); ok {
				// Hash + Hash: merge-style; right wins on key clash.
				return callMethod(env, lh, "merge", []object.RubyObject{rh})
			}
		}
	case "-":
		if larr, ok := left.(*object.Array); ok {
			if rarr, ok := right.(*object.Array); ok {
				out := []object.RubyObject{}
			nextElem:
				for _, e := range larr.Elements {
					for _, x := range rarr.Elements {
						if rubyEqual(e, x) {
							continue nextElem
						}
					}
					out = append(out, e)
				}
				return object.NewArray(out...), nil
			}
		}
	case "*":
		if larr, ok := left.(*object.Array); ok {
			if n, ok := right.(*object.Integer); ok {
				if n.Value < 0 {
					return nil, errorf("evaluator: ArgumentError: negative array repeat")
				}
				out := make([]object.RubyObject, 0, len(larr.Elements)*int(n.Value))
				for i := int64(0); i < n.Value; i++ {
					out = append(out, larr.Elements...)
				}
				return object.NewArray(out...), nil
			}
			if sep, ok := stringText(env, right); ok {
				return callMethod(env, larr, "join", []object.RubyObject{object.NewString(sep)})
			}
		}
	case "&":
		if larr, ok := left.(*object.Array); ok {
			if rarr, ok := right.(*object.Array); ok {
				out := []object.RubyObject{}
				for _, e := range larr.Elements {
					for _, x := range rarr.Elements {
						if rubyEqual(e, x) {
							// dedupe
							seen := false
							for _, k := range out {
								if rubyEqual(k, e) {
									seen = true
									break
								}
							}
							if !seen {
								out = append(out, e)
							}
							break
						}
					}
				}
				return object.NewArray(out...), nil
			}
		}
	case "|":
		if larr, ok := left.(*object.Array); ok {
			if rarr, ok := right.(*object.Array); ok {
				out := []object.RubyObject{}
				add := func(e object.RubyObject) {
					for _, k := range out {
						if rubyEqual(k, e) {
							return
						}
					}
					out = append(out, e)
				}
				for _, e := range larr.Elements {
					add(e)
				}
				for _, e := range rarr.Elements {
					add(e)
				}
				return object.NewArray(out...), nil
			}
		}
		if s, ok := left.(*object.String); ok {
			if t, ok := stringText(env, right); ok {
				s.Buf = append(s.Buf, t...)
				return s, nil
			}
			if i, ok := right.(*object.Integer); ok {
				s.Buf = append(s.Buf, string(rune(i.Value))...)
				return s, nil
			}
		}
	}

	if v, handled, err := numericInfix(env, n.Operator, left, right); handled {
		if _, isZD := err.(zeroDivErr); isZD {
			return raiseBuiltin(env, "ZeroDivisionError", "divided by 0")
		}
		return v, err
	}

	if v, handled, err := stringInfix(env, n.Operator, left, right); handled {
		return v, err
	}

	return nil, errorf("evaluator: unsupported infix %q for %T / %T", n.Operator, left, right)
}

func evalIndex(env *object.Environment, n *ast.IndexExpression) (object.RubyObject, error) {
	recv, err := Eval(n.Left, env)
	if err != nil {
		return nil, err
	}
	args, err := evalExpressions(env, n.Arguments)
	if err != nil {
		return nil, err
	}
	switch r := recv.(type) {
	case *object.Hash:
		if len(args) != 1 {
			return nil, errorf("evaluator: Hash#[] needs 1 arg, got %d", len(args))
		}
		for _, e := range r.Entries {
			if rubyEqualDispatch(env, e.Key, args[0]) {
				return e.Value, nil
			}
		}
		if r.DefaultBlock != nil {
			if p, ok := r.DefaultBlock.(*object.Proc); ok {
				return invokeProc(env, p, []object.RubyObject{r, args[0]})
			}
		}
		if r.Default != nil {
			return r.Default, nil
		}
		return object.NIL, nil
	case *object.Array:
		return arrayIndex(r, args)
	case *object.Proc:
		// Proc#[] is sugar for #call: `p[a, b]` invokes the proc with
		// the given args. Same dispatch as `.()` / `.call`.
		return invokeProc(env, r, args)
	case *object.Instance:
		if m, found := dispatchClass(env, r).LookupMethod("[]"); found {
			if um, ok := m.(*object.UserMethod); ok {
				return invokeMethodOn(env, r, um, args, nil)
			}
		}
		return nil, errorf("evaluator: NoMethodError: undefined method `[]' for instance of %s", r.C.Name)
	case *object.Class:
		// User-defined class-level [] (def self.[](...) ... end).
		// Dispatch through ClassMethods first so a user definition
		// can replace the builtin shortcuts below.
		if _, ok := r.LookupClassMethod("[]"); ok {
			return callMethod(env, r, "[]", args)
		}
		// `Hash[...]` / `Array[...]` constructor shorthand.
		switch r.Name {
		case "Hash":
			return hashClassIndex(env, args)
		case "Array":
			return object.NewArray(args...), nil
		}
		return nil, errorf("evaluator: %s[] not supported", r.Name)
	}
	if s, ok := stringText(env, recv); ok {
		return stringIndex(s, args)
	}
	return nil, errorf("evaluator: [] not yet supported on %T", recv)
}

// stringIndex implements String#[] for the corpus shapes:
// s[i], s[i, n], s[range].
func stringIndex(s string, args []object.RubyObject) (object.RubyObject, error) {
	runes := []rune(s)
	switch len(args) {
	case 1:
		switch k := args[0].(type) {
		case *object.Integer:
			i := int(k.Value)
			if i < 0 {
				i += len(runes)
			}
			if i < 0 || i >= len(runes) {
				return object.NIL, nil
			}
			return object.NewString(string(runes[i])), nil
		case *object.Range:
			lo, hi, ok := rangeBounds(k, len(runes))
			if !ok {
				return object.NIL, nil
			}
			return object.NewString(string(runes[lo:hi])), nil
		case *object.Regex:
			// String#[regex] -- returns the matched substring or nil. Used
			// by code like `ARGV[0][/^-d=(.)/]` to test+extract in one shot.
			// Note: MRI returns the full match (group 0). To pick a group,
			// MRI takes two args (regex + group index) -- handled in the
			// 2-arg branch below.
			if m := k.RE.FindString(s); m != "" {
				return object.NewString(m), nil
			}
			// FindString returns "" both for "no match" and "matched the
			// empty string". Disambiguate via Find:
			if loc := k.RE.FindStringIndex(s); loc != nil {
				return object.NewString(""), nil
			}
			return object.NIL, nil
		case *object.String, *object.FrozenString:
			// `"hello"["ll"]` returns the matched substring or nil.
			t := ""
			switch v := k.(type) {
			case *object.String:
				t = string(v.Buf)
			case *object.FrozenString:
				t = "<frozen>" // placeholder; never hit in current paths
				_ = v
			}
			if strings.Contains(s, t) {
				return object.NewString(t), nil
			}
			return object.NIL, nil
		}
		return nil, errorf("evaluator: String#[] unsupported index type %T", args[0])
	case 2:
		// (regex, group_index) -- pick capture group from match.
		if re, ok := args[0].(*object.Regex); ok {
			g, ok := args[1].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: String#[regex, group] group must be Integer")
			}
			loc := re.RE.FindStringSubmatchIndex(s)
			if loc == nil {
				return object.NIL, nil
			}
			idx := int(g.Value)
			if idx < 0 || idx*2+1 >= len(loc) {
				return object.NIL, nil
			}
			if loc[idx*2] < 0 {
				return object.NIL, nil
			}
			return object.NewString(s[loc[idx*2]:loc[idx*2+1]]), nil
		}
		start, ok1 := args[0].(*object.Integer)
		count, ok2 := args[1].(*object.Integer)
		if !ok1 || !ok2 {
			return nil, errorf("evaluator: String#[start, len] needs Integer args")
		}
		st := int(start.Value)
		if st < 0 {
			st += len(runes)
		}
		if st < 0 || st > len(runes) || count.Value < 0 {
			return object.NIL, nil
		}
		end := st + int(count.Value)
		if end > len(runes) {
			end = len(runes)
		}
		return object.NewString(string(runes[st:end])), nil
	}
	return nil, errorf("evaluator: String#[] with %d args not supported", len(args))
}

// arrayIndex implements Array#[] for the forms the corpus uses:
// a[i], a[i, n], a[range].
func arrayIndex(a *object.Array, args []object.RubyObject) (object.RubyObject, error) {
	switch len(args) {
	case 1:
		switch k := args[0].(type) {
		case *object.Integer:
			idx := int(k.Value)
			if idx < 0 {
				idx += len(a.Elements)
			}
			if idx < 0 || idx >= len(a.Elements) {
				return object.NIL, nil
			}
			return a.Elements[idx], nil
		case *object.Range:
			lo, hi, ok := rangeBounds(k, len(a.Elements))
			if !ok {
				return object.NIL, nil
			}
			out := make([]object.RubyObject, hi-lo)
			copy(out, a.Elements[lo:hi])
			return object.NewArray(out...), nil
		}
		return nil, errorf("evaluator: Array#[] unsupported index type %T", args[0])
	case 2:
		start, ok1 := args[0].(*object.Integer)
		count, ok2 := args[1].(*object.Integer)
		if !ok1 || !ok2 {
			return nil, errorf("evaluator: Array#[start, len] needs Integer args")
		}
		s := int(start.Value)
		if s < 0 {
			s += len(a.Elements)
		}
		if s < 0 || s > len(a.Elements) || count.Value < 0 {
			return object.NIL, nil
		}
		end := s + int(count.Value)
		if end > len(a.Elements) {
			end = len(a.Elements)
		}
		out := make([]object.RubyObject, end-s)
		copy(out, a.Elements[s:end])
		return object.NewArray(out...), nil
	}
	return nil, errorf("evaluator: Array#[] with %d args not supported", len(args))
}

// hashClassIndex implements `Hash[...]` -- the class-level subscript
// shorthand. Accepts either a single Array of pairs (`Hash[[[k,v],
// ...]]`) or an even-length flat arg list (`Hash[k, v, k, v]`).
func hashClassIndex(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) == 1 {
		if arr, ok := args[0].(*object.Array); ok {
			entries := make([]object.HashEntry, 0, len(arr.Elements))
			for _, e := range arr.Elements {
				pair, ok := e.(*object.Array)
				if !ok || len(pair.Elements) != 2 {
					return nil, errorf("evaluator: Hash[]: array element must be 2-element Array")
				}
				entries = append(entries, object.HashEntry{Key: pair.Elements[0], Value: pair.Elements[1]})
			}
			return object.NewHash(entries...), nil
		}
		if h, ok := args[0].(*object.Hash); ok {
			return h, nil
		}
	}
	if len(args)%2 != 0 {
		return nil, errorf("evaluator: ArgumentError: odd number of arguments for Hash[]")
	}
	entries := make([]object.HashEntry, 0, len(args)/2)
	for i := 0; i < len(args); i += 2 {
		entries = append(entries, object.HashEntry{Key: args[i], Value: args[i+1]})
	}
	_ = env
	return object.NewHash(entries...), nil
}

// rangeToSlice materialises a Range to its element list. Supports
// Integer bounds (the fast path) and String bounds (via String#succ).
func rangeToSlice(env *object.Environment, r *object.Range) ([]object.RubyObject, error) {
	if lo, hi, ok := rangeIntegerBounds(r); ok {
		end := hi
		if !r.Exclusive {
			end = hi + 1
		}
		out := make([]object.RubyObject, 0, end-lo)
		for i := lo; i < end; i++ {
			out = append(out, object.NewInteger(i))
		}
		return out, nil
	}
	lb, lok := stringText(env, r.Begin)
	hb, hok := stringText(env, r.End)
	if lok && hok {
		out := []object.RubyObject{}
		cur := lb
		for {
			if r.Exclusive && cur == hb {
				break
			}
			out = append(out, object.NewString(cur))
			if !r.Exclusive && cur == hb {
				break
			}
			// Cap pathological ranges (e.g. shorter end) to avoid
			// runaway loops.
			if len(cur) > len(hb) {
				break
			}
			next := stringSucc(cur)
			if next == cur {
				break
			}
			cur = next
		}
		return out, nil
	}
	return nil, errorf("evaluator: Range over %T not iterable", r.Begin)
}

// rangeIntegerBounds returns the (begin, end) integer values of a
// Range when both ends are Integer literals. Used by Range#each /
// Range#map; the inclusive-vs-exclusive choice is left to callers.
func rangeIntegerBounds(r *object.Range) (int64, int64, bool) {
	b, bOK := r.Begin.(*object.Integer)
	e, eOK := r.End.(*object.Integer)
	if !bOK || !eOK {
		return 0, 0, false
	}
	return b.Value, e.Value, true
}

// rangeBounds resolves a Range against a sequence of n elements,
// returning [lo, hi) clamped to bounds. Returns ok=false for ranges
// whose Begin lies outside [0, n].
func rangeBounds(r *object.Range, n int) (lo, hi int, ok bool) {
	b, bOK := r.Begin.(*object.Integer)
	e, eOK := r.End.(*object.Integer)
	if !bOK || !eOK {
		return 0, 0, false
	}
	lo = int(b.Value)
	if lo < 0 {
		lo += n
	}
	if lo < 0 || lo > n {
		return 0, 0, false
	}
	hi = int(e.Value)
	if hi < 0 {
		hi += n
	}
	if r.Exclusive {
		// `a...b` excludes b
	} else {
		hi++
	}
	if hi > n {
		hi = n
	}
	if hi < lo {
		hi = lo
	}
	return lo, hi, true
}

// evalDefined implements `defined?(expr)`: returns a String describing
// the kind of thing the expression refers to, or nil when undefined.
// Matches the common return values MRI emits for the shapes that
// actually appear in user code (locals, methods, ivars, globals,
// constants, expressions).
func evalDefined(env *object.Environment, n *ast.DefinedExpression) (object.RubyObject, error) {
	switch e := n.Expr.(type) {
	case *ast.Identifier:
		if isConstantName(e.Value) {
			if cls := env.EnclosingClass(); cls != nil {
				if _, ok := lookupConstant(cls, e.Value); ok {
					return object.NewString("constant"), nil
				}
			}
			if _, ok := env.Get(e.Value); ok {
				return object.NewString("constant"), nil
			}
			return object.NIL, nil
		}
		if _, ok := env.Get(e.Value); ok {
			return object.NewString("local-variable"), nil
		}
		if _, ok := env.GetMethod(e.Value); ok {
			return object.NewString("method"), nil
		}
		return object.NIL, nil
	case *ast.InstanceVariable:
		self := env.EnclosingSelf()
		if inst, ok := self.(*object.Instance); ok {
			if _, found := inst.Ivars["@"+e.Name.Value]; found {
				return object.NewString("instance-variable"), nil
			}
		}
		return object.NIL, nil
	case *ast.Global:
		if _, ok := env.Get(e.Value); ok {
			return object.NewString("global-variable"), nil
		}
		return object.NIL, nil
	case *ast.ContextCallExpression:
		// Probe by attempting eval; any error -> undefined.
		if _, err := Eval(e, env); err == nil {
			return object.NewString("method"), nil
		}
		return object.NIL, nil
	}
	// Generic expression: evaluate; success means "expression", error
	// means undefined.
	if _, err := Eval(n.Expr, env); err == nil {
		return object.NewString("expression"), nil
	}
	return object.NIL, nil
}

// truthy follows ruby semantics: only nil and false are falsy.
func truthy(o object.RubyObject) bool {
	switch v := o.(type) {
	case *object.Nil:
		return false
	case *object.Boolean:
		return v.Value
	}
	return true
}

// rubyEqual implements ruby `==` between common runtime types. Returns
// false for incomparable type pairs (matching MRI's default Object#==).
// Numerics cross-equate (3 == 3.0 is true).
// rubyEqualDispatch is rubyEqual extended with a fallback to the
// receiver's user-defined `==` method. Use this when the comparison
// happens *inside* method-dispatch operations (Array#include?,
// Hash#has_key?, case/when), where MRI consults the receiver class's
// `==`. Pure rubyEqual stays value-only so it can be called from
// places that have no env handy.
func rubyEqualDispatch(env *object.Environment, a, b object.RubyObject) bool {
	if rubyEqual(a, b) {
		return true
	}
	inst, ok := a.(*object.Instance)
	if !ok {
		return false
	}
	m, found := dispatchClass(env, inst).LookupMethod("==")
	if !found {
		return false
	}
	um, ok := m.(*object.UserMethod)
	if !ok {
		return false
	}
	res, err := invokeMethodOn(env, inst, um, []object.RubyObject{b}, nil)
	if err != nil {
		return false
	}
	return truthy(res)
}

func rubyEqual(a, b object.RubyObject) bool {
	switch x := a.(type) {
	case *object.Integer:
		if y, ok := b.(*object.Integer); ok {
			if x.IsBig() || y.IsBig() {
				return x.ToBig().Cmp(y.ToBig()) == 0
			}
			return x.Value == y.Value
		}
		if y, ok := b.(*object.Float); ok {
			if x.IsBig() {
				lf, _ := new(big.Float).SetInt(x.Bn).Float64()
				return lf == y.Value
			}
			return float64(x.Value) == y.Value
		}
		return false
	case *object.Float:
		if y, ok := b.(*object.Float); ok {
			return x.Value == y.Value
		}
		if y, ok := b.(*object.Integer); ok {
			return x.Value == float64(y.Value)
		}
		return false
	case *object.Symbol:
		y, ok := b.(*object.Symbol)
		return ok && x.ID == y.ID
	case *object.String:
		y, ok := b.(*object.String)
		return ok && string(x.Buf) == string(y.Buf)
	case *object.Boolean:
		y, ok := b.(*object.Boolean)
		return ok && x.Value == y.Value
	case *object.Nil:
		_, ok := b.(*object.Nil)
		return ok
	case *object.Array:
		y, ok := b.(*object.Array)
		if !ok || len(x.Elements) != len(y.Elements) {
			return false
		}
		for i := range x.Elements {
			if !rubyEqual(x.Elements[i], y.Elements[i]) {
				return false
			}
		}
		return true
	case *object.Hash:
		y, ok := b.(*object.Hash)
		if !ok || len(x.Entries) != len(y.Entries) {
			return false
		}
		for _, e := range x.Entries {
			found := false
			for _, f := range y.Entries {
				if rubyEqual(e.Key, f.Key) && rubyEqual(e.Value, f.Value) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	}
	return false
}
