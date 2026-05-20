package object

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
}

func (m *UserMethod) Type() Type       { return OBJECT_OBJ }
func (m *UserMethod) Class() RubyClass { return nil }
func (m *UserMethod) Inspect() string  { return "#<Method:" + m.Name + ">" }
