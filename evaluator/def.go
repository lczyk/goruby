package evaluator

import (
	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/object"
)

// returnSignal is raised through the call by ReturnStatement and caught
// at the method-call frame, which unwraps it into the return value.
type returnSignal struct {
	Value object.RubyObject
}

func (r *returnSignal) Error() string { return "unhandled return" }

// evalFunctionLiteral registers the method on the root env and returns
// its name as a Symbol (matching MRI's `def` return value).
func evalFunctionLiteral(env *object.Environment, n *ast.FunctionLiteral) (object.RubyObject, error) {
	if n.IsLambda {
		return procFromLambda(env, n), nil
	}
	for _, p := range n.Parameters {
		switch {
		case p.IsNoKeywords, p.IsForwarding, p.IsImplicitRest:
			return nil, errorf("evaluator: parameter kind on %q not yet supported", p.Name.Value)
		}
	}
	m := &object.UserMethod{
		Name:          n.Name.Value,
		Body:          n.Body,
		Params:        n.Parameters,
		CapturedBlock: n.CapturedBlock,
		DefEnv:        env,
		Rescues:       n.Rescues,
		ElseBody:      n.ElseBody,
		EnsureBody:    n.EnsureBody,
		Endless:       n.IsEndless,
	}

	// `def self.foo` (Receiver.Value == "self") -> class method on the
	// enclosing class. `def foo` inside a class body -> instance
	// method. `def foo` at top level -> kernel-style method on root env.
	if n.Receiver != nil {
		if n.Receiver.Value != "self" {
			return nil, errorf("evaluator: singleton-method def on receiver %q not yet supported", n.Receiver.Value)
		}
		cls := env.EnclosingClass()
		if cls == nil {
			return nil, errorf("evaluator: def self.%s used outside a class body", n.Name.Value)
		}
		cls.AddClassMethod(n.Name.Value, m)
		return env.Symbols().Intern(n.Name.Value), nil
	}

	if cls := env.EnclosingClass(); cls != nil {
		cls.AddMethod(n.Name.Value, m)
		return env.Symbols().Intern(n.Name.Value), nil
	}

	env.SetMethod(n.Name.Value, m)
	return env.Symbols().Intern(n.Name.Value), nil
}

// callUserMethod binds args to params in a fresh enclosed env (so
// globals stay reachable but locals don't leak), then evaluates the
// body. Catches returnSignal raised by `return`.
func callUserMethod(env *object.Environment, m *object.UserMethod, args []object.RubyObject) (object.RubyObject, error) {
	return callUserMethodWithBlock(env, m, args, nil)
}

// callUserMethodWithProc dispatches a top-level user method with a
// Proc supplied via `&block` capture. The Proc is bundled into a
// goBlockMarker so `yield` / `&blk` capture inside the method body
// route through invokeProc.
func callUserMethodWithProc(env *object.Environment, m *object.UserMethod, args []object.RubyObject, p *object.Proc) (object.RubyObject, error) {
	defEnv, _ := m.DefEnv.(*object.Environment)
	if defEnv == nil {
		defEnv = env
	}
	callEnv := object.NewEnclosedEnvironment(defEnv)
	callEnv.MethodFrame = true
	callEnv.CurrentMethodName = m.Name
	callEnv.CurrentMethodArgs = args
	callEnv.CurrentKwargs = env.CurrentKwargs
	callEnv.CurrentBlock = &goBlockMarker{fn: func(a []object.RubyObject) (object.RubyObject, error) {
		return invokeProc(env, p, a)
	}}
	return runMethodBody(callEnv, m, args)
}

func callUserMethodWithBlock(env *object.Environment, m *object.UserMethod, args []object.RubyObject, blk *ast.BlockExpression) (object.RubyObject, error) {
	defEnv, _ := m.DefEnv.(*object.Environment)
	if defEnv == nil {
		defEnv = env
	}

	callEnv := object.NewEnclosedEnvironment(defEnv)
	callEnv.MethodFrame = true
	callEnv.CurrentMethodName = m.Name
	callEnv.CurrentMethodArgs = args
	callEnv.CurrentKwargs = env.CurrentKwargs
	if blk != nil {
		callEnv.CurrentBlock = blk
	}
	return runMethodBody(callEnv, m, args)
}

// hasKeywordParam reports whether any param is keyword-shaped.
func hasKeywordParam(params []*ast.FunctionParameter) bool {
	for _, p := range params {
		if p.IsKeyword || p.IsKeywordRest {
			return true
		}
	}
	return false
}

// runMethodBody binds params and evaluates the method body in callEnv
// (assumed pre-configured with MethodFrame and any Self / CurrentBlock).
// Catches the returnSignal raised by `return`.
func runMethodBody(callEnv *object.Environment, m *object.UserMethod, args []object.RubyObject) (object.RubyObject, error) {
	if v, ok, err := dispatchAttrMarker(callEnv, m, args); ok {
		return v, err
	}

	params, _ := m.Params.([]*ast.FunctionParameter)
	body, _ := m.Body.(*ast.BlockStatement)

	// MRI <3.0 semantics: when the callee has only positional params
	// (no IsKeyword/IsKeywordRest) but the caller supplied kwargs,
	// bundle them into a trailing Hash arg. Mirrors `f(name: "x")`
	// passing `{name: "x"}` to `def f(opts)`.
	if len(callEnv.CurrentKwargs) > 0 && !hasKeywordParam(params) {
		entries := make([]object.HashEntry, 0, len(callEnv.CurrentKwargs))
		for k, v := range callEnv.CurrentKwargs {
			entries = append(entries, object.HashEntry{
				Key:   callEnv.Symbols().Intern(k),
				Value: v,
			})
		}
		args = append(args, object.NewHash(entries...))
		callEnv.CurrentKwargs = nil
	}

	if err := bindParams(callEnv, params, args); err != nil {
		return nil, err
	}

	if cap, ok := m.CapturedBlock.(*ast.BlockCapture); ok && cap != nil && cap.Name != nil {
		var bound object.RubyObject = object.NIL
		if blkAny := callEnv.CurrentBlock; blkAny != nil {
			switch blk := blkAny.(type) {
			case *ast.BlockExpression:
				bound = procFromBlock(callEnv.Outer(), blk)
			case *goBlockMarker:
				bound = procFromGoBlock(blk)
			}
		}
		callEnv.Set(cap.Name.Value, bound)
	}

	result, err := evalBlockStatement(callEnv, body)

	// Method-body rescue / else / ensure: wraps the body in the same
	// dispatch as ExceptionHandlingBlock so `def foo; ...; rescue Foo
	// => e; ...; end` works without an explicit begin/end.
	rescues, _ := m.Rescues.([]*ast.RescueBlock)
	elseBody, _ := m.ElseBody.(*ast.BlockStatement)
	ensureBody, _ := m.EnsureBody.(*ast.BlockStatement)

	if err == nil && elseBody != nil {
		result, err = evalBlockStatement(callEnv, elseBody)
	}

	if err != nil {
		if rs, ok := err.(*returnSignal); ok {
			result = rs.Value
			err = nil
		} else if rs, ok := err.(*raiseSignal); ok && len(rescues) > 0 {
			handled := false
			for _, r := range rescues {
				match, mErr := rescueMatches(callEnv, r, rs.Exception)
				if mErr != nil {
					err = mErr
					break
				}
				if !match {
					continue
				}
				if r.Exception != nil {
					callEnv.AssignVisible(r.Exception.Value, rs.Exception)
				}
				result, err = evalBlockStatement(callEnv, r.Body)
				handled = true
				break
			}
			if !handled {
				err = rs
			}
		}
	}

	if ensureBody != nil {
		if _, eerr := evalBlockStatement(callEnv, ensureBody); eerr != nil {
			err = eerr
		}
	}

	if err != nil {
		return nil, err
	}
	return result, nil
}

// dispatchAttrMarker recognises the synthesised attr_reader /
// attr_writer methods produced by makeAttrReader / makeAttrWriter and
// reads / writes the corresponding ivar on the receiver.
func dispatchAttrMarker(callEnv *object.Environment, m *object.UserMethod, args []object.RubyObject) (object.RubyObject, bool, error) {
	switch marker := m.Body.(type) {
	case structInitMarker:
		inst, ok := callEnv.Self.(*object.Instance)
		if !ok {
			return nil, true, errorf("evaluator: Struct initialize on non-instance %T", callEnv.Self)
		}
		for i, f := range []string(marker) {
			var v object.RubyObject = object.NIL
			if i < len(args) {
				v = args[i]
			}
			inst.Ivars["@"+f] = v
		}
		return object.NIL, true, nil
	case structToAMarker:
		inst, ok := callEnv.Self.(*object.Instance)
		if !ok {
			return nil, true, errorf("evaluator: Struct#to_a on non-instance %T", callEnv.Self)
		}
		out := make([]object.RubyObject, 0, len(marker))
		for _, f := range []string(marker) {
			if v, ok := inst.Ivars["@"+f]; ok {
				out = append(out, v)
			} else {
				out = append(out, object.NIL)
			}
		}
		return object.NewArray(out...), true, nil
	case structMembersMarker:
		out := make([]object.RubyObject, 0, len(marker))
		for _, f := range []string(marker) {
			out = append(out, callEnv.Symbols().Intern(f))
		}
		return object.NewArray(out...), true, nil
	case dataInitMarker:
		inst, ok := callEnv.Self.(*object.Instance)
		if !ok {
			return nil, true, errorf("evaluator: Data initialize on non-instance %T", callEnv.Self)
		}
		kwargs := callEnv.CurrentKwargs
		for _, f := range []string(marker) {
			v, ok := kwargs[f]
			if !ok {
				return nil, true, errorf("evaluator: ArgumentError: missing keyword: :%s", f)
			}
			inst.Ivars["@"+f] = v
		}
		return object.NIL, true, nil
	case exceptionMessageMarker:
		_ = marker
		inst, ok := callEnv.Self.(*object.Instance)
		if !ok {
			return nil, true, errorf("evaluator: message called on non-instance %T", callEnv.Self)
		}
		if v, ok := inst.Ivars["@message"]; ok {
			return v, true, nil
		}
		return object.NewString(inst.C.Name), true, nil
	case exceptionInitMarker:
		_ = marker
		inst, ok := callEnv.Self.(*object.Instance)
		if !ok {
			return nil, true, errorf("evaluator: Exception#initialize on non-instance %T", callEnv.Self)
		}
		msg := inst.C.Name
		if len(args) >= 1 {
			if s, ok := stringText(callEnv, args[0]); ok {
				msg = s
			}
		}
		inst.Ivars["@message"] = object.NewString(msg)
		return object.NIL, true, nil
	case nativeFn:
		v, err := marker.fn(callEnv, args)
		return v, true, err
	case mathFn1:
		if len(args) != 1 {
			return nil, true, errorf("evaluator: wrong number of arguments (given %d, expected 1)", len(args))
		}
		f, err := toFloatValue(args[0])
		if err != nil {
			return nil, true, err
		}
		return object.NewFloat(marker.fn(f)), true, nil
	case mathFn2:
		if len(args) != 2 {
			return nil, true, errorf("evaluator: wrong number of arguments (given %d, expected 2)", len(args))
		}
		a, err := toFloatValue(args[0])
		if err != nil {
			return nil, true, err
		}
		b, err := toFloatValue(args[1])
		if err != nil {
			return nil, true, err
		}
		return object.NewFloat(marker.fn(a, b)), true, nil
	case attrReaderMarker:
		inst, ok := callEnv.Self.(*object.Instance)
		if !ok {
			return nil, true, errorf("evaluator: attr reader on non-instance receiver %T", callEnv.Self)
		}
		if v, ok := inst.Ivars["@"+string(marker)]; ok {
			return v, true, nil
		}
		return object.NIL, true, nil
	case attrWriterMarker:
		inst, ok := callEnv.Self.(*object.Instance)
		if !ok {
			return nil, true, errorf("evaluator: attr writer on non-instance receiver %T", callEnv.Self)
		}
		if len(args) != 1 {
			return nil, true, errorf("evaluator: attr writer expected 1 arg, got %d", len(args))
		}
		inst.Ivars["@"+string(marker)] = args[0]
		return args[0], true, nil
	}
	return nil, false, nil
}

// bindParams matches a positional arg list to a parameter list, applying
// defaults for missing slots and collecting trailing args into a splat
// when present.
func bindParams(env *object.Environment, params []*ast.FunctionParameter, args []object.RubyObject) error {
	positional := make([]*ast.FunctionParameter, 0, len(params))
	keywords := make([]*ast.FunctionParameter, 0)
	for _, p := range params {
		if p.IsKeyword || p.IsKeywordRest {
			keywords = append(keywords, p)
			continue
		}
		positional = append(positional, p)
	}

	if err := bindPositionalParams(env, positional, args); err != nil {
		return err
	}
	return bindKeywordParams(env, keywords)
}

func bindKeywordParams(env *object.Environment, kws []*ast.FunctionParameter) error {
	if len(kws) == 0 {
		return nil
	}
	supplied := env.CurrentKwargs
	// Split keyword-rest (**opts) -- only one per signature.
	var rest *ast.FunctionParameter
	consumed := map[string]bool{}
	for _, p := range kws {
		if p.IsKeywordRest {
			rest = p
			continue
		}
		if v, ok := supplied[p.Name.Value]; ok {
			env.Set(p.Name.Value, v)
			consumed[p.Name.Value] = true
			continue
		}
		if p.Default != nil {
			v, err := Eval(p.Default, env)
			if err != nil {
				return err
			}
			env.Set(p.Name.Value, v)
			continue
		}
		return errorf("evaluator: ArgumentError: missing keyword: :%s", p.Name.Value)
	}
	if rest != nil && rest.Name != nil {
		entries := []object.HashEntry{}
		// Preserve insertion order from CurrentKwargs by re-iterating
		// the original map; map iteration order isn't stable in Go,
		// but kwargs map insertion order isn't preserved anyway --
		// good enough for the corpus.
		for k, v := range supplied {
			if consumed[k] {
				continue
			}
			entries = append(entries, object.HashEntry{
				Key:   env.Symbols().Intern(k),
				Value: v,
			})
		}
		env.Set(rest.Name.Value, object.NewHash(entries...))
	}
	return nil
}

func bindPositionalParams(env *object.Environment, params []*ast.FunctionParameter, args []object.RubyObject) error {
	splatIdx := -1
	for i, p := range params {
		if p.IsSplat {
			splatIdx = i
			break
		}
	}

	if splatIdx == -1 {
		required := 0
		for _, p := range params {
			if p.Default == nil {
				required++
			}
		}
		if len(args) < required || len(args) > len(params) {
			return errorf("evaluator: ArgumentError: wrong number of arguments (given %d, expected %d)", len(args), len(params))
		}
		ai := 0
		for _, p := range params {
			if ai < len(args) {
				env.Set(p.Name.Value, args[ai])
				ai++
				continue
			}
			v, err := Eval(p.Default, env)
			if err != nil {
				return err
			}
			env.Set(p.Name.Value, v)
		}
		return nil
	}

	preCount := splatIdx
	postCount := len(params) - splatIdx - 1
	if len(args) < preCount+postCount {
		return errorf("evaluator: ArgumentError: wrong number of arguments (given %d, expected %d+)", len(args), preCount+postCount)
	}
	for i := 0; i < preCount; i++ {
		env.Set(params[i].Name.Value, args[i])
	}
	splatLen := len(args) - preCount - postCount
	splatArgs := make([]object.RubyObject, splatLen)
	copy(splatArgs, args[preCount:preCount+splatLen])
	env.Set(params[splatIdx].Name.Value, object.NewArray(splatArgs...))
	for i := 0; i < postCount; i++ {
		env.Set(params[splatIdx+1+i].Name.Value, args[preCount+splatLen+i])
	}
	return nil
}

// breakSignal unwinds out of the current loop / block iteration's
// owning loop, optionally with a value (which becomes the loop's
// result).
type breakSignal struct{ Value object.RubyObject }

func (b *breakSignal) Error() string { return "unhandled break" }

// nextSignal short-circuits the current iteration of a loop / block,
// with an optional value handed to the block's caller as the
// iteration's value (used e.g. by Array#map).
type nextSignal struct{ Value object.RubyObject }

func (n *nextSignal) Error() string { return "unhandled next" }

// evalJump handles break/next/redo/retry. The parser also routes
// modifier-form return through a JumpExpression whose token is RETURN.
func evalJump(env *object.Environment, n *ast.JumpExpression) (object.RubyObject, error) {
	kind := n.Token.Type.Literal()
	switch kind {
	case "return":
		var v object.RubyObject = object.NIL
		if n.Value != nil {
			got, err := Eval(n.Value, env)
			if err != nil {
				return nil, err
			}
			v = expandSingle(got)
		}
		return nil, &returnSignal{Value: v}
	case "break":
		var v object.RubyObject = object.NIL
		if n.Value != nil {
			got, err := Eval(n.Value, env)
			if err != nil {
				return nil, err
			}
			v = expandSingle(got)
		}
		return nil, &breakSignal{Value: v}
	case "next":
		var v object.RubyObject = object.NIL
		if n.Value != nil {
			got, err := Eval(n.Value, env)
			if err != nil {
				return nil, err
			}
			v = expandSingle(got)
		}
		return nil, &nextSignal{Value: v}
	}
	return nil, errorf("evaluator: %s not yet supported", kind)
}

func evalReturn(env *object.Environment, n *ast.ReturnStatement) (object.RubyObject, error) {
	var v object.RubyObject = object.NIL
	if n.ReturnValue != nil {
		got, err := Eval(n.ReturnValue, env)
		if err != nil {
			return nil, err
		}
		v = expandSingle(got)
	}
	return nil, &returnSignal{Value: v}
}
