package transformer

import (
	"fmt"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/trace"
)

func Transform(node *ast.Program, trace_transform bool) (*ast.Program, error) {
	t := NewTransformer().(*transformer)
	if trace_transform {
		t.tracer = trace.NewTracer()
	}
	transformed_node, err := t.Transform(node)
	if err != nil {
		return nil, err
	}
	node, ok := transformed_node.(*ast.Program)
	if !ok {
		return nil, fmt.Errorf("expected *ast.Program, got %T", transformed_node)
	}
	return node, nil
}

type Transformer interface {
	Transform(node ast.Node) (ast.Node, error)
}

func NewTransformer() Transformer {
	return &transformer{}
}

var (
	_ Transformer = &transformer{}
)
