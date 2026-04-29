package transformer

import (
	"fmt"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/trace"
)

type transformer struct {
	tracer trace.Tracer
}

func (t *transformer) Transform(node ast.Node) (ast.Node, error) {
	program, is_program := node.(*ast.Program)

	var transformed ast.Node
	var out_err error

	if is_program {
		transformed, out_err = t.transformProgram(program)

	} else {
		panic("Transforming non-program node is not yet implemented")
	}

	return transformed, out_err
}

type call struct {
	name       string
	parameters []string
}

func (c *call) String() string {
	return fmt.Sprintf("call(name=%s, parameters=%v)", c.name, c.parameters)
}

type block_lifting_transformer struct {
	call_stack []*call
	// functions which we lifted and want to now stick in the top level of the program
	lifted_functions []*ast.FunctionLiteral
	// change the names of some of the functions
	name_changes map[string]string // map of old name to new name
	// pass number. this is a two-pass transformation.
	pass int
}

func (f *block_lifting_transformer) PreTransform(node ast.Node) ast.Node {
	switch node := node.(type) {
	case nil:
		//
	case *ast.Comment:
		//
	case *ast.FunctionLiteral:
		// increment the call stack
		parameter_names := make([]string, len(node.Parameters))
		for i, param := range node.Parameters {
			parameter_names[i] = param.Name
		}
		f.call_stack = append(f.call_stack, &call{name: node.Name, parameters: parameter_names})
		return node
	default:
		fmt.Printf("# TRANSFORMER walking %T\n", node)
	}
	return node
}

func (f *block_lifting_transformer) PostTransform(node ast.Node) ast.Node {
	switch node := node.(type) {
	case nil:
		//
	case *ast.Comment:
		//
	case *ast.FunctionLiteral:
		// decrement the call stack
		if len(f.call_stack) > 0 {
			f.call_stack = f.call_stack[:len(f.call_stack)-1]
		}
		// call the post-transform function
		if f.pass == 0 {
			return f.transformFunctionLiteralPass0(node)
		}
	case *ast.ContextCallExpression:
		if f.pass == 1 {
			return f.transformContextCallExpressionPass1(node)
		}
	default:
		fmt.Printf("# TRANSFORMER walking %T\n", node)
	}
	return node
}

func (f *block_lifting_transformer) transformFunctionLiteralPass0(node *ast.FunctionLiteral) ast.Node {
	if len(f.call_stack) > 0 {
		parent := f.call_stack[len(f.call_stack)-1]

		// And add all the parameters from the parent
		extra_parameters := make([]*ast.FunctionParameter, 0)

		// TODO!
		// NOTE: This won't really work that well. What if we define a local variable
		// and need to capture that?

		for _, param := range parent.parameters {
			function_parameter := &ast.FunctionParameter{Name: param}
			extra_parameters = append(extra_parameters, function_parameter)
		}

		var lifted_function *ast.FunctionLiteral
		var replacement_node *ast.IndexExpression
		if len(extra_parameters) > 0 {
			// we need a factory function
			factory_body := &ast.BlockStatement{
				Statements: []ast.Statement{
					&ast.ExpressionStatement{
						Expression: node,
					},
				},
			}
			lifted_function = &ast.FunctionLiteral{
				Name:       node.Name,
				Parameters: extra_parameters,
				Body:       factory_body,
			}
			var index ast.ExpressionList
			for _, param := range parent.parameters {
				index = append(index, &ast.Identifier{Value: param})
			}
			replacement_node = &ast.IndexExpression{
				Left:  &ast.Identifier{Value: node.Name},
				Index: index,
			}
		} else {
			// lift the function as is just the node
			lifted_function = node
			var index ast.ExpressionList
			for _, param := range node.Parameters {
				index = append(index, &ast.Identifier{Value: param.Name})
			}
			for _, param := range parent.parameters {
				index = append(index, &ast.Identifier{Value: param})
			}
			replacement_node = &ast.IndexExpression{
				Left:  &ast.Identifier{Value: node.Name},
				Index: index,
			}
		}

		f.lifted_functions = append(f.lifted_functions, lifted_function)

		// return replacement_node
		return replacement_node
	} else {
		// modified_node := node
		old_name := node.Name
		node.Name = fmt.Sprintf("__grgr_lifted_%s", node.Name)
		// return &ast.Assignment{
		// 	Left:  &ast.Identifier{Value: old_name},
		// 	Right: modified_node,
		// }
		if f.name_changes == nil {
			f.name_changes = make(map[string]string)
		}
		f.name_changes[old_name] = node.Name
		f.lifted_functions = append(f.lifted_functions, node)
		return &ast.IntegerLiteral{Value: 42} // just a placeholder, we don't lift the function
	}
}

func (f *block_lifting_transformer) transformContextCallExpressionPass1(node *ast.ContextCallExpression) ast.Node {
	if node.Context == nil {
		new_name, ok := f.name_changes[node.Function]
		if ok {
			// node.Function = new_name
			new_node := &ast.IndexExpression{
				Left:  &ast.Identifier{Value: new_name},
				Index: ast.ExpressionList(node.Arguments),
			}
			return new_node
		}
	}
	return node
}

func (f *block_lifting_transformer) TransformerMarker() {}

var (
	_ ast.PreTransformer = &block_lifting_transformer{}
	_ ast.Transformer    = &block_lifting_transformer{}
)

////////////////////////////////////////////////////////////////////////////////

type lift_transformer struct {
	statements []ast.Statement // to be lifted
}

func (f *lift_transformer) PostTransform(node ast.Node) ast.Node {
	switch node := node.(type) {
	case nil:
		//
	case *ast.Comment:
		//
	case *ast.ContextCallExpression:
		// fmt.Printf("%v (%T)\n", node, node)
		fmt.Printf("# TRANSFORMER found context call expression \"%s\"\n", node.Function)
		// f.functions = append(f.functions, node)

		var replacement replacement_spec
		replacement, ok := context_call_expr_replacements[context_spec(node.Function)]
		if ok {
			// found a replacement !!
			f.statements = append(f.statements, replacement.statement)
			return &ast.IndexExpression{
				Left: &ast.Identifier{Value: replacement.name},
				Index: ast.ExpressionList{
					node.Context,
					node.Block,
				},
			}
		}
	default:
		fmt.Printf("# TRANSFORMER walking %T\n", node)
	}
	return node
}

func (f *lift_transformer) TransformerMarker() {}

var (
	_ ast.PostTransformer = &lift_transformer{}
	_ ast.Transformer     = &lift_transformer{}
)

////////////////////////////////////////////////////////////////////////////////

func (t *transformer) transformProgram(program *ast.Program) (*ast.Program, error) {
	// var new_statement ast.Statement = context_call_expr_replacements[context_spec("find_all")].statement
	// program.Statements = append([]ast.Statement{new_statement}, program.Statements...)

	const ENABLE_BLOCK_LIFT_TRANSFORMER = true
	// const ENABLE_BLOCK_LIFT_TRANSFORMER = false
	if ENABLE_BLOCK_LIFT_TRANSFORMER {
		transformer := &block_lifting_transformer{}
		fmt.Printf("# TRANSFORMER applying %T pass 0\n", transformer)
		ast.Walk(program, transformer, nil)
		(*transformer).pass = 1
		fmt.Printf("# TRANSFORMER applying %T pass 1\n", transformer)
		ast.Walk(program, transformer, nil)
		if len(transformer.lifted_functions) > 0 {
			var lifted []ast.Statement = make([]ast.Statement, len(transformer.lifted_functions))
			for _, function := range transformer.lifted_functions {
				lifted = append(lifted, &ast.ExpressionStatement{
					Expression: &ast.Assignment{
						Left:  &ast.Identifier{Value: function.Name},
						Right: function,
					},
				})
			}
			program.Statements = append(lifted, program.Statements...)
		}

		fmt.Printf("# TRANSFORMER done with %T\n", transformer)
	}

	const ENABLE_LIFT_TRANSFORMER = true
	// const ENABLE_LIFT_TRANSFORMER = false
	if ENABLE_LIFT_TRANSFORMER {
		var transformer = &lift_transformer{}
		fmt.Printf("# TRANSFORMER applying %T\n", transformer)
		_ = ast.Walk(program, transformer, nil)
		if len(transformer.statements) > 0 {
			program.Statements = append(transformer.statements, program.Statements...)
		}
		fmt.Printf("# TRANSFORMER done with %T\n", transformer)
	}

	return program, nil
}
