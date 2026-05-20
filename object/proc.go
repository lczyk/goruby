package object

// Proc wraps a callable block (lambda literal, `&blk`-captured block,
// or `Proc.new { ... }`). Body / Params live as `any` so this package
// needn't import ast.
type Proc struct {
	Params   any // []*ast.FunctionParameter
	Body     any // *ast.BlockStatement
	DefEnv   any // *Environment captured at creation time
	IsLambda bool
}

func (p *Proc) Type() Type       { return PROC_OBJ }
func (p *Proc) Class() RubyClass { return nil }
func (p *Proc) Inspect() string {
	if p.IsLambda {
		return "#<Proc (lambda)>"
	}
	return "#<Proc>"
}
