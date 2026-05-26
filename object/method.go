package object

// RubyMethod is the dispatch-side interface every callable method (user
// or builtin) implements. The block arg is typed as `any` so this
// package needn't import ast: the evaluator passes either
// `*ast.BlockExpression` or nil.
type RubyMethod interface {
	Call(env *Environment, recv RubyObject, args []RubyObject, block any) (RubyObject, error)
}

// UserMethodInvoker is the evaluator-supplied hook that runs a user
// method's AST body. Set at package init from the evaluator to break
// the import cycle (object can't import evaluator). nil = no invoker
// registered, which is a programmer error if hit.
var UserMethodInvoker func(env *Environment, recv RubyObject, m *UserMethod, args []RubyObject, block any) (RubyObject, error)

// UserMethod wraps a ruby-source method definition so the evaluator can
// dispatch calls back into the AST body. The Body / Params / DefEnv
// fields are kept as `any` so this package needn't import ast or
// re-export the evaluator's environment-wrapping types.
type UserMethod struct {
	Name          string
	Body          any // *ast.BlockStatement
	Params        any // []*ast.FunctionParameter
	CapturedBlock any // *ast.BlockCapture, when the def declares `&blk`
	DefEnv        any // *Environment, captured at def time
	Rescues       any // []*ast.RescueBlock, method-body rescue clauses
	ElseBody      any // *ast.BlockStatement
	EnsureBody    any // *ast.BlockStatement
	Endless       bool
	// SourceFile records the absolute path of the source file the def
	// was lexed from. Used to populate env.CurrentFile when the method
	// body runs, so errors raised inside the body report against the
	// definition's source -- not whichever file happened to be active
	// when the method was *called* (after a chain of require_relative
	// + cross-file dispatches).
	SourceFile string
	// DefClass records the class/module the method was defined inside.
	// Used to anchor `super` lookups so that a method defined in a
	// module mixed into a class climbs strictly later in the ancestry
	// (rather than re-finding itself through the includer's chain).
	// Nil for toplevel defs.
	DefClass *Class
}

func (m *UserMethod) Type() Type       { return OBJECT_OBJ }
func (m *UserMethod) Class() RubyClass { return nil }
func (m *UserMethod) Inspect() string  { return "#<Method:" + m.Name + ">" }

func (m *UserMethod) Call(env *Environment, recv RubyObject, args []RubyObject, block any) (RubyObject, error) {
	return UserMethodInvoker(env, recv, m, args, block)
}

// BuiltinMethod wraps a Go function as a RubyMethod, used to attach
// native implementations to builtin classes (Integer#+, String#upcase,
// etc.) as the dispatcher migrates off its hand-rolled switch.
type BuiltinMethod struct {
	Name string
	Fn   func(env *Environment, recv RubyObject, args []RubyObject, block any) (RubyObject, error)
}

func (b *BuiltinMethod) Call(env *Environment, recv RubyObject, args []RubyObject, block any) (RubyObject, error) {
	return b.Fn(env, recv, args, block)
}
