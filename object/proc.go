package object

// Proc wraps a callable block (lambda literal, `&blk`-captured block,
// or `Proc.new { ... }`). Body / Params live as `any` so this package
// needn't import ast.
type Proc struct {
	Params   any // []*ast.FunctionParameter
	Body     any // *ast.BlockStatement
	DefEnv   any // *Environment captured at creation time
	IsLambda bool
	// IsMethod marks Procs returned by Object#method(:name). They
	// dispatch the same way (the bound-method marker in Params still
	// drives invocation) but their Class() returns MethodClass for
	// MRI-compatible introspection.
	IsMethod bool
}

func (p *Proc) Type() Type { return PROC_OBJ }
func (p *Proc) Class() RubyClass {
	if p.IsMethod {
		return MethodClass
	}
	return ProcClass
}
func (p *Proc) Inspect() string {
	if p.IsMethod {
		return "#<Method>"
	}
	if p.IsLambda {
		return "#<Proc (lambda)>"
	}
	return "#<Proc>"
}
