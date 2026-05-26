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
		SourceFile:    env.CurrentFile(),
		DefClass:      env.EnclosingClass(),
	}

	// `def self.foo` (Receiver.Value == "self") -> class method on the
	// enclosing class. `def foo` inside a class body -> instance
	// method. `def foo` at top level -> kernel-style method on root env.
	if n.Receiver != nil {
		if n.Receiver.Value == "self" {
			cls := env.EnclosingClass()
			if cls == nil {
				return nil, errorf("evaluator: def self.%s used outside a class body", n.Name.Value)
			}
			cls.AddClassMethod(n.Name.Value, m)
			return env.Symbols().Intern(n.Name.Value), nil
		}
		// Singleton-method def: `def obj.foo` attaches the method to
		// the receiver value's per-object SingletonMethods table.
		// Receiver must already be bound in scope. Class receivers
		// install as class methods; Instance receivers get per-object
		// methods that win over the class chain in Send.
		recv, ok := env.Get(n.Receiver.Value)
		if !ok {
			// Receiver might be a Constant defined on an enclosing
			// class/module. Walk the lexical class chain and check
			// each class's Constants table before bailing.
			if isConstantName(n.Receiver.Value) {
				for cur := env; cur != nil; cur = cur.Outer() {
					if cls := cur.CurrentClass; cls != nil {
						if v, found := cls.Constants[n.Receiver.Value]; found {
							recv = v
							ok = true
							break
						}
					}
				}
			}
		}
		// Ivar receiver: `def @app.foo` -- look up @app on the enclosing
		// self's instance variables. Rake's test suite uses this shape
		// extensively to stub methods on per-test fixtures.
		if !ok && len(n.Receiver.Value) > 0 && n.Receiver.Value[0] == '@' {
			if self := env.EnclosingSelf(); self != nil {
				if inst, isInst := self.(*object.Instance); isInst {
					if v, found := inst.Ivars[n.Receiver.Value]; found {
						recv = v
						ok = true
					}
				}
			}
		}
		if !ok {
			return nil, errorf("evaluator: singleton def: undefined receiver %q", n.Receiver.Value)
		}
		switch r := recv.(type) {
		case *object.Class:
			r.AddClassMethod(n.Name.Value, m)
		case *object.Instance:
			if r.SingletonMethods == nil {
				r.SingletonMethods = map[string]object.RubyMethod{}
			}
			r.SingletonMethods[n.Name.Value] = m
		default:
			return nil, errorf("evaluator: singleton def on %T not yet supported", recv)
		}
		return env.Symbols().Intern(n.Name.Value), nil
	}

	// `class << host ... def foo ...` installs onto host's singleton
	// table. Checked before EnclosingClass so a singleton block inside
	// a class body still routes correctly.
	if host := env.EnclosingSingletonHost(); host != nil {
		if err := installSingletonMethod(host, n.Name.Value, m); err != nil {
			return nil, err
		}
		return env.Symbols().Intern(n.Name.Value), nil
	}

	if cls := env.EnclosingClass(); cls != nil {
		cls.AddMethod(n.Name.Value, m)
		switch cls.CurrentVisibility {
		case "private":
			if cls.Private == nil {
				cls.Private = map[string]bool{}
			}
			cls.Private[n.Name.Value] = true
		case "protected":
			if cls.Protected == nil {
				cls.Protected = map[string]bool{}
			}
			cls.Protected[n.Name.Value] = true
		case "module_function":
			// Also install as a class method so Module.method works.
			// Instance copy stays private per MRI.
			cls.AddClassMethod(n.Name.Value, m)
			if cls.Private == nil {
				cls.Private = map[string]bool{}
			}
			cls.Private[n.Name.Value] = true
		}
		return env.Symbols().Intern(n.Name.Value), nil
	}

	env.SetMethod(n.Name.Value, m)
	return env.Symbols().Intern(n.Name.Value), nil
}

// installSingletonMethod registers m on host's per-object method table.
// Class -> ClassMethods (so a class-level singleton def matches
// `def self.foo`); Instance -> SingletonMethods (per-object).
func installSingletonMethod(host object.RubyObject, name string, m object.RubyMethod) error {
	switch h := host.(type) {
	case *object.Class:
		h.AddClassMethod(name, m)
	case *object.Instance:
		if h.SingletonMethods == nil {
			h.SingletonMethods = map[string]object.RubyMethod{}
		}
		h.SingletonMethods[name] = m
	default:
		return errorf("evaluator: singleton def on %T not yet supported", host)
	}
	return nil
}

// evalAlias implements `alias new_name old_name`: registers the
// method old_name's implementation under new_name on the enclosing
// class (or as a top-level method when there is no enclosing class).
// Mirrors mri's alias: a snapshot of the current resolution -- later
// redefinition of old_name doesn't affect the alias.
func evalAlias(env *object.Environment, n *ast.AliasExpression) (object.RubyObject, error) {
	if n.NewName == nil || n.OldName == nil {
		return nil, errorf("evaluator: alias: missing name")
	}
	newN, oldN := n.NewName.Value, n.OldName.Value
	if cls := env.EnclosingClass(); cls != nil {
		m, ok := cls.LookupMethod(oldN)
		if !ok {
			return nil, errorf("evaluator: NameError: undefined method `%s' for class `%s'", oldN, cls.Name)
		}
		cls.Methods[newN] = m
		return object.NIL, nil
	}
	m, ok := env.GetMethod(oldN)
	if !ok {
		return nil, errorf("evaluator: NameError: undefined method `%s' for main:Object", oldN)
	}
	env.SetMethod(newN, m)
	return object.NIL, nil
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
	if m.SourceFile != "" {
		prev := env.SetCurrentFile(m.SourceFile)
		defer env.SetCurrentFile(prev)
	}
	pushMethodFrame(env, m)
	defer env.PopCallFrame()
	return runMethodBody(callEnv, m, args)
}

// pushMethodFrame records the called method on the call stack. MRI's
// backtrace lists frames innermost-first including the raising
// method; Kernel#caller drops the topmost (current method) and lists
// outward. So we push the callee's name on entry and pop on return;
// Kernel#caller skips the head, exception backtrace reads it all.
func pushMethodFrame(env *object.Environment, m *object.UserMethod) {
	sf := m.SourceFile
	if sf == "" {
		sf = "(eval)"
	}
	env.PushCallFrame(sf + ":0:in `" + m.Name + "'")
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

	// Pre-declare locals introduced anywhere in the body so a read
	// inside a branch that didn't run still sees nil (MRI's parser-
	// time lvar introduction). Without this, the Ruby idiom
	//   if cond; x = ...; end
	//   x || default
	// trips a NameError on `x` when cond is false. rake/task.rb's
	// lookup_prerequisite is the motivating shape.
	if body != nil {
		for _, s := range body.Statements {
			predeclareNode(callEnv, s)
		}
	}

	if cap, ok := m.CapturedBlock.(*ast.BlockCapture); ok && cap != nil && cap.Name != nil {
		var bound object.RubyObject = object.NIL
		if blkAny := callEnv.CurrentBlock; blkAny != nil {
			switch blk := blkAny.(type) {
			case *ast.BlockExpression:
				if blk != nil {
					bound = procFromBlock(callEnv.Outer(), blk)
				}
			case *goBlockMarker:
				if blk != nil {
					bound = procFromGoBlock(blk)
				}
			}
		}
		callEnv.Set(cap.Name.Value, bound)
	}

	// Method-body rescue / else / ensure: wraps the body in the same
	// dispatch as ExceptionHandlingBlock so `def foo; ...; rescue Foo
	// => e; ...; end` works without an explicit begin/end.
	rescues, _ := m.Rescues.([]*ast.RescueBlock)
	elseBody, _ := m.ElseBody.(*ast.BlockStatement)
	ensureBody, _ := m.EnsureBody.(*ast.BlockStatement)

	var result object.RubyObject
	var err error
	for {
		result, err = evalBlockStatement(callEnv, body)

		if err == nil && elseBody != nil {
			result, err = evalBlockStatement(callEnv, elseBody)
		}

		retried := false
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
					prevExc := callEnv.CurrentException
					callEnv.CurrentException = rs.Exception
					result, err = evalBlockStatement(callEnv, r.Body)
					callEnv.CurrentException = prevExc
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
		}
		if !retried {
			break
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
		v, err := marker.Fn(callEnv, args)
		return v, true, err
	case attrReaderMarker:
		key := "@" + string(marker)
		switch self := callEnv.Self.(type) {
		case *object.Instance:
			if v, ok := self.Ivars[key]; ok {
				return v, true, nil
			}
			return object.NIL, true, nil
		case *object.Class:
			// class-level attr accessor (installed via `class << self;
			// attr_accessor :x; end` in a class/module body). Reads the
			// class-instance variable rather than an Instance's ivar.
			if v, ok := self.Ivars[key]; ok {
				return v, true, nil
			}
			return object.NIL, true, nil
		}
		return nil, true, errorf("evaluator: attr reader on non-instance receiver %T", callEnv.Self)
	case attrWriterMarker:
		if len(args) != 1 {
			return nil, true, errorf("evaluator: attr writer expected 1 arg, got %d", len(args))
		}
		key := "@" + string(marker)
		switch self := callEnv.Self.(type) {
		case *object.Instance:
			self.Ivars[key] = args[0]
			return args[0], true, nil
		case *object.Class:
			if self.Ivars == nil {
				self.Ivars = map[string]object.RubyObject{}
			}
			self.Ivars[key] = args[0]
			return args[0], true, nil
		}
		return nil, true, errorf("evaluator: attr writer on non-instance receiver %T", callEnv.Self)
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
			return errorf("evaluator: ArgumentError: wrong number of arguments (given %d, expected %d) for %s", len(args), len(params), env.CurrentMethodName)
		}
		ai := 0
		for _, p := range params {
			if ai < len(args) {
				if err := bindOneParam(env, p, args[ai]); err != nil {
					return err
				}
				ai++
				continue
			}
			v, err := Eval(p.Default, env)
			if err != nil {
				return err
			}
			if err := bindOneParam(env, p, v); err != nil {
				return err
			}
		}
		return nil
	}

	preCount := splatIdx
	postCount := len(params) - splatIdx - 1
	if len(args) < preCount+postCount {
		return errorf("evaluator: ArgumentError: wrong number of arguments (given %d, expected %d+)", len(args), preCount+postCount)
	}
	for i := 0; i < preCount; i++ {
		if err := bindOneParam(env, params[i], args[i]); err != nil {
			return err
		}
	}
	splatLen := len(args) - preCount - postCount
	splatArgs := make([]object.RubyObject, splatLen)
	copy(splatArgs, args[preCount:preCount+splatLen])
	// Anonymous splat (`*` with no name): consume args, don't bind.
	if params[splatIdx].Name != nil {
		env.Set(params[splatIdx].Name.Value, object.NewArray(splatArgs...))
	}
	for i := 0; i < postCount; i++ {
		if err := bindOneParam(env, params[splatIdx+1+i], args[preCount+splatLen+i]); err != nil {
			return err
		}
	}
	return nil
}

// bindOneParam binds value to param's name, recursing into nested
// destructuring (|(a, b), c| ...) when param.Destructure is non-nil.
// Destructuring requires the value to be an Array (or convertible);
// short arrays pad with nil, extras drop -- matches ruby's block-
// arity tolerance for tuple destructuring.
func bindOneParam(env *object.Environment, param *ast.FunctionParameter, value object.RubyObject) error {
	if len(param.Destructure) == 0 {
		env.Set(param.Name.Value, value)
		return nil
	}
	arr, ok := value.(*object.Array)
	var elems []object.RubyObject
	if ok {
		elems = arr.Elements
	} else {
		elems = []object.RubyObject{value}
	}
	for i, inner := range param.Destructure {
		var v object.RubyObject = object.NIL
		if i < len(elems) {
			v = elems[i]
		}
		if err := bindOneParam(env, inner, v); err != nil {
			return err
		}
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

// retrySignal flows out of a rescue clause to re-run the
// surrounding begin body. Caught by the rescue handler in
// runMethodBody / ExceptionHandlingBlock; reaches further out
// only if no rescue context catches it (which is an error).
type retrySignal struct{}

func (r *retrySignal) Error() string { return "retry" }

// throwSignal flows out of throw to the matching catch frame.
// Caught by Kernel#catch when the Tag matches; propagates
// otherwise.
type throwSignal struct {
	Tag   object.RubyObject
	Value object.RubyObject
}

func (t *throwSignal) Error() string { return "throw" }

func (n *nextSignal) Error() string { return "unhandled next" }

// exitSignal short-circuits the entire program. Raised by Kernel#exit
// and Kernel#exit!; caught at the top-level Eval so tests don't tear
// down the process via os.Exit.
type exitSignal struct{ Code int64 }

func (e *exitSignal) Error() string { return "unhandled exit" }

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
	case "retry":
		return nil, &retrySignal{}
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
