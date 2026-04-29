package evaluator_test

import (
	"go/token"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/compare"
	"github.com/lczyk/goruby/evaluator"
	"github.com/lczyk/goruby/object"
	"github.com/lczyk/goruby/parser"
	"github.com/pkg/errors"
)

func TestEvalComment(t *testing.T) {
	input := "5 # five"

	evaluated, err := testEval(input)
	assert.NoError(t, err)

	var expected int64 = 5
	testIntegerObject(t, evaluated, expected)
}

func TestEvalIntegerExpression(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"5", 5},
		{"10", 10},
		{"-5", -5},
		{"-10", -10},
		{"5 + 5 + 5 + 5 - 10", 10},
		{"2 * 2 * 2 * 2 * 2", 32},
		{"-50 + 100 + -50", 0},
		{"5 * 2 + 10", 20},
		{"5 + 2 * 10", 25},
		{"20 + 2 * -10", 0},
		{"50 / 2 * 2 + 10", 60},
		{"2 * (5 + 10)", 30},
		{"3 * 3 * 3 + 10", 37},
		{"3 * (3 * 3) + 10", 37},
		{"(5 + 10 * 2 + 15 / 3) * 2 + -10", 50},
		{"5 % 2", 1},
	}

	for _, tt := range tests {
		evaluated, err := testEval(tt.input, object.NewMainEnvironment())
		assert.NoError(t, err)
		testIntegerObject(t, evaluated, tt.expected)
	}
}

func TestEvalBooleanExpression(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"true", true},
		{"false", false},
		{"1 < 2", true},
		{"1 > 2", false},
		{"1 < 1", false},
		{"1 > 1", false},
		{"1 == 1", true},
		{"1 != 1", false},
		{"1 == 2", false},
		{"1 != 2", true},
		{"true == true", true},
		{"false == false", true},
		{"true == false", false},
		{"true != false", true},
		{"false != true", true},
		{"(1 < 2) == true", true},
		{"(1 < 2) == false", false},
		{"(1 > 2) == true", false},
		{"(1 > 2) == false", true},
		{"true || true", true},
		{"false || false", false},
		{"true || false", true},
		{"false || false", false},
	}

	for _, tt := range tests {
		evaluated, err := testEval(tt.input)
		assert.NoError(t, err)
		testBooleanObject(t, evaluated, tt.expected)
	}
}

func TestBangOperator(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"!true", false},
		{"!false", true},
		{"!5", false},
		{"!!true", true},
		{"!!false", false},
		{"!!5", true},
	}

	for _, tt := range tests {
		evaluated, err := testEval(tt.input)
		assert.NoError(t, err)
		testBooleanObject(t, evaluated, tt.expected)
	}
}

func TestConditionalExpression(t *testing.T) {
	tests := []struct {
		input    string
		expected interface{}
	}{
		{"if true; 10; end", 10},
		{"if false; 10; end", nil},
		{"if 1; 10; end", 10},
		{"if 1 < 2; 10; end", 10},
		{"if 1 > 2; 10; end", nil},
		{"if 1 > 2; 10; else\n 20; end", 20},
		{"if 1 < 2; 10; else\n 20; end", 10},
		{"unless true; 10; end", nil},
		{"unless false; 10; end", 10},
		{"unless 1; 10; end", nil},
		{"unless 1 < 2; 10; end", nil},
		{"unless 1 > 2; 10; end", 10},
		{"unless 1 > 2; 10; else\n 20; end", 10},
		{"unless 1 < 2; 10; else\n 20; end", 20},
	}

	for _, tt := range tests {
		evaluated, err := testEval(tt.input)
		assert.NoError(t, err)
		integer, ok := tt.expected.(int)
		if ok {
			testIntegerObject(t, evaluated, int64(integer))
		} else {
			testNilObject(t, evaluated)
		}
	}
}

func TestReturnStatements(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"return 10;", 10},
		{"return 10; 9;", 10},
		{"return 2 * 5; 9;", 10},
		{"9; return 2 * 5; 9;", 10},
		{`if 10 > 1
			if 10 > 1
				return 10
			end
			return 1
		  end`, 10},
	}

	for _, tt := range tests {
		evaluated, err := testEval(tt.input)
		assert.NoError(t, err)
		testIntegerObject(t, evaluated, tt.expected)
	}
}

func TestErrorHandling(t *testing.T) {
	tests := []struct {
		input           string
		expectedMessage string
	}{
		{
			"5 + true;",
			"TypeError: Integer can't be coerced into Symbol",
		},
		{
			"5 + true; 5;",
			"TypeError: Integer can't be coerced into Symbol",
		},
		{
			"-true",
			"Exception: unknown operator: -Symbol",
		},
		{
			"true + false;",
			"NoMethodError: undefined method `+' for :true:Symbol",
		},
		{
			"true + false + true + false;",
			"NoMethodError: undefined method `+' for :true:Symbol",
		},
		{
			"5; true + false; 5",
			"NoMethodError: undefined method `+' for :true:Symbol",
		},
		{
			`"Hello" - "World"`,
			"NoMethodError: undefined method `-' for Hello:String",
		},
		{
			"if (10 > 1); true + false; end",
			"NoMethodError: undefined method `+' for :true:Symbol",
		},
		{
			"if (10 > 1); true + false; end",
			"NoMethodError: undefined method `+' for :true:Symbol",
		},
		{
			`
if (10 > 1)
	if (10 > 1)
		return true + false;
	end
	return 1;
end
`,
			"NoMethodError: undefined method `+' for :true:Symbol",
		},
		{
			"foobar",
			"NoMethodError: undefined method `foobar' for :funcs:Symbol",
		},
		{
			"Foobar",
			"NameError: uninitialized constant Foobar",
		},
		{
			`
			def foo x, y
			end

			foo 1
			`,
			"ArgumentError: wrong number of arguments (given 1, expected 2)",
		},
	}

	for _, tt := range tests {
		env := object.NewEnvironment()
		_, err := testEval(tt.input, env)
		assert.NotEqual(t, err, nil)

		actual, ok := errors.Cause(err).(object.RubyObject)
		assert.That(t, ok, "Error is not a RubyObject. got=%T (%+v)", err, err)
		assert.That(t, object.IsError(actual), "Expected error or exception, got %T", actual)
		assert.Equal(t, actual.Inspect(), tt.expectedMessage)
	}
}

func TestAssignment(t *testing.T) {
	t.Run("assign to hash", func(t *testing.T) {
		tests := []struct {
			name     string
			input    string
			expected interface{}
		}{
			{
				"return value for anonymous hash",
				`{:foo => 3}[:foo] = 5`,
				5,
			},
			{
				"return value for hash variable",
				`x = {:foo => 3}; x[:foo] = 5`,
				5,
			},
			{
				"variable value after hash assignment",
				`x = {:foo => 3}; x[:foo] = 5; x`,
				map[string]string{":foo": "5"},
			},
		}

		for _, tt := range tests {
			evaluated, err := testEval(tt.input)
			assert.NoError(t, err)
			testObject(t, evaluated, tt.expected)
		}
	})
	t.Run("assign to local variable", func(t *testing.T) {
		tests := []struct {
			input    string
			expected int64
		}{
			{
				`foo = 5`,
				5,
			},
			{
				`foo = 5; x = foo; x = 3; x`,
				3,
			},
			{"a = 5; a;", 5},
			{"a = 5 * 5; a;", 25},
			{"a = 5; b = a; b;", 5},
			{"a = 5; b = a; c = a + b + 5; c;", 15},
		}

		for _, tt := range tests {
			evaluated, err := testEval(tt.input, object.NewMainEnvironment())
			assert.NoError(t, err)
			testIntegerObject(t, evaluated, tt.expected)
		}
	})
	t.Run("assign more than one value", func(t *testing.T) {
		tests := []struct {
			input    string
			expected interface{}
		}{
			{
				`foo = 5, 4`,
				[]string{"5", "4"},
			},
			{
				`foo = 5, 4; foo`,
				[]string{"5", "4"},
			},
		}

		for _, tt := range tests {
			evaluated, err := testEval(tt.input, object.NewMainEnvironment())
			assert.NoError(t, err)
			testObject(t, evaluated, tt.expected)
		}
	})
	t.Run("assign to array", func(t *testing.T) {
		tests := []struct {
			input    string
			elements []object.RubyObject
		}{
			{
				`x = [3]; x[0] = 5; x`,
				[]object.RubyObject{object.NewInteger(5)},
			},
			{
				`x = []; x[0] = 5; x`,
				[]object.RubyObject{object.NewInteger(5)},
			},
			{
				`x = [3]; x[3] = 5; x`,
				[]object.RubyObject{object.NewInteger(3), object.NIL, object.NIL, object.NewInteger(5)},
			},
		}

		for _, tt := range tests {
			evaluated, err := testEval(tt.input)
			assert.NoError(t, err)

			array, ok := evaluated.(*object.Array)
			assert.That(t, ok, "object is not Array. got=%T (%+v)", evaluated, evaluated)
			assert.Equal(t, len(array.Elements), len(tt.elements))
			assert.EqualCmpAny(t, array.Elements, tt.elements, object.CompareRubyObjectsForTests)
		}
	})
	t.Run("assign operator on local variable", func(t *testing.T) {
		tests := []struct {
			input    string
			expected int64
		}{
			{
				`foo = 2; foo += 5`,
				7,
			},
			{
				`foo = 5; foo -= 3; foo`,
				2,
			},
		}

		for _, tt := range tests {
			evaluated, err := testEval(tt.input, object.NewMainEnvironment())
			assert.NoError(t, err)
			testIntegerObject(t, evaluated, tt.expected)
		}
	})
}

func TestMultiAssignment(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output *object.Array
	}{
		{
			name:  "evenly distributed sides",
			input: "x, y, z = 1, 2, 3; [x, y, z]",
			output: object.NewArray(
				object.NewInteger(1),
				object.NewInteger(2),
				object.NewInteger(3),
			),
		},
		{
			name:  "value side one less",
			input: "x, y, z = 1, 2; [x, y, z]",
			output: object.NewArray(
				object.NewInteger(1),
				object.NewInteger(2),
				object.NIL,
			),
		},
		{
			name:  "value side two less",
			input: "x, y, z = 1; [x, y, z]",
			output: object.NewArray(
				object.NewInteger(1),
				object.NIL,
				object.NIL,
			),
		},
		{
			name:  "lhs with global and const",
			input: "$x, Y = 1, 2; [$x, Y]",
			output: object.NewArray(
				object.NewInteger(1),
				object.NewInteger(2),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluated, err := testEval(tt.input, object.NewMainEnvironment())
			assert.NoError(t, err)
			assert.EqualCmpAny(t, evaluated, tt.output, object.CompareRubyObjectsForTests)
		})
	}
}

func TestGlobalAssignmentExpression(t *testing.T) {
	t.Run("assignments", func(t *testing.T) {
		tests := []struct {
			input    string
			expected int64
		}{
			{"$a = 5; $a;", 5},
			{"$a = 5 * 5; $a;", 25},
			{"$a = 5; $b = $a; $b;", 5},
			{"$a = 5; $b = $a; $c = $a + $b + 5; $c;", 15},
		}

		for _, tt := range tests {
			t.Run(tt.input, func(t *testing.T) {
				evaluated, err := testEval(tt.input)
				assert.NoError(t, err)
				testIntegerObject(t, evaluated, tt.expected)
			})
		}
	})
	t.Run("set as global", func(t *testing.T) {
		input := "$Foo = 3"

		outer := object.NewEnvironment()
		env := object.NewEnclosedEnvironment(outer)
		_, err := testEval(input, env)
		assert.NoError(t, err)

		_, ok := outer.Get("$Foo")
		assert.That(t, ok, "Expected $FOO to be set in outer env, was not")

		_, ok = env.Clone().Get("$Foo")
		assert.That(t, !ok, "Expected $FOO to not be set in inner env, was")
	})
}

func TestFunctionObject(t *testing.T) {
	type funcParam struct {
		name         string
		defaultValue object.RubyObject
	}
	t.Run("methods without receiver", func(t *testing.T) {
		tests := []struct {
			input              string
			expectedParameters []funcParam
			expectedCode       string
		}{
			{
				"def foo x; x + 2; end",
				[]funcParam{{name: "x"}},
				"x + 2",
			},
			{
				`def foo
				2
				end`,
				[]funcParam{},
				"2",
			},
			{
				"def foo; 2; end",
				[]funcParam{},
				"2",
			},
			{
				"def foo x = 4; 2; end",
				[]funcParam{{name: "x", defaultValue: object.NewInteger(4)}},
				"2",
			},
		}

		for _, tt := range tests {
			env := object.NewEnvironment()
			evaluated, err := testEval(tt.input, env)
			assert.NoError(t, err)
			sym, ok := evaluated.(*object.Symbol)
			assert.That(t, ok, "object is not Symbol. got=%T (%+v)", evaluated, evaluated)
			assert.Equal(t, sym.Value, "foo")

			method, ok := object.FUNCS_STORE.Class().Methods().Get("foo")
			assert.That(t, ok, "Expected method to be added to self")
			fn, ok := method.(*object.Function)
			assert.That(t, ok, "Expected method to be a function. got=%T (%+v)", method, method)
			assert.Equal(t, len(fn.Parameters), len(tt.expectedParameters))

			for i, param := range fn.Parameters {
				testParam := tt.expectedParameters[i]
				assert.Equal(t, param.Name, param.Name)
				assert.EqualCmpAny(t, param.Default, testParam.defaultValue, object.CompareRubyObjectsForTests)
			}

			assert.Equal(t, fn.Body.Code(), tt.expectedCode)
		}
	})
}

func TestFunctionApplication(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"def identity x; x; end; identity(5);", 5},
		{"def identity x; return x; end; identity(5);", 5},
		{"def double x; x * 2; end; double(5);", 10},
		{"def add x, y; x + y; end; add(5, 5);", 10},
		{"def add x, y; x + y; end; add(5 + 5, add(5, 5));", 20},
		{"def double x; x * 2; end; double 5;", 10},
		{"def identity x; x; end; identity 5;", 5},
		{"def foo; 3; end; foo;", 3},
	}

	for _, tt := range tests {
		env := object.NewEnvironment()
		evaluated, err := testEval(tt.input, env)
		assert.NoError(t, err)
		testIntegerObject(t, evaluated, tt.expected)
	}
}

func TestGlobalLiteral(t *testing.T) {
	input := `$foo = 'bar'; $foo`

	evaluated, err := testEval(input)
	assert.NoError(t, err)
	str, ok := evaluated.(*object.String)
	assert.That(t, ok, "object is not String. got=%T (%+v)", evaluated, evaluated)
	assert.Equal(t, str.Value, "bar")
}

func TestStringLiteral(t *testing.T) {
	input := `"Hello World!"`

	evaluated, err := testEval(input)
	assert.NoError(t, err)
	str, ok := evaluated.(*object.String)
	assert.That(t, ok, "object is not String. got=%T (%+v)", evaluated, evaluated)
	assert.Equal(t, str.Value, "Hello World!")
}

func TestStringConcatenation(t *testing.T) {
	input := `"Hello" + " " + "World!"`

	evaluated, err := testEval(input)
	assert.NoError(t, err)
	str, ok := evaluated.(*object.String)
	assert.That(t, ok, "object is not String. got=%T (%+v)", evaluated, evaluated)
	assert.Equal(t, str.Value, "Hello World!")
}

func TestSymbolLiteral(t *testing.T) {
	input := `:foobar;`

	evaluated, err := testEval(input)
	assert.NoError(t, err)
	sym, ok := evaluated.(*object.Symbol)
	assert.That(t, ok, "object is not Symbol. got=%T (%+v)", evaluated, evaluated)
	assert.Equal(t, sym.Value, "foobar")
}

func TestMethodCalls(t *testing.T) {
	t.Skip("TODO: check we get a correct error")
	// input := "x = 2; x.foo :bar"
	// _, _ := testEval(input)
	// assert.Error(t, err, object.NewNoMethodError("undefined method `foo' for 2:Integer"))
}

func TestArrayLiterals(t *testing.T) {
	input := "[1, 2 * 2, 3 + 3]"
	evaluated, err := testEval(input)
	assert.NoError(t, err)

	result, ok := evaluated.(*object.Array)
	assert.That(t, ok, "object is not Array. got=%T (%+v)", evaluated, evaluated)
	assert.Equal(t, len(result.Elements), 3)
	testIntegerObject(t, result.Elements[0], 1)
	testIntegerObject(t, result.Elements[1], 4)
	testIntegerObject(t, result.Elements[2], 6)
}

func TestArrayIndexExpressions(t *testing.T) {
	tests := []struct {
		input    string
		expected interface{}
	}{
		{
			"[1, 2, 3][0]",
			1,
		},
		{
			"[1, 2, 3][1]",
			2,
		},
		{
			"[1, 2, 3][2]",
			3,
		},
		{
			"i = 0; [1][i];",
			1,
		},
		{
			"[1, 2, 3][1 + 1];",
			3,
		},
		{
			"myArray = [1, 2, 3]; myArray[2];",
			3,
		},
		{
			"myArray = [1, 2, 3]; myArray[0] + myArray[1] + myArray[2];",
			6,
		},
		{
			"myArray = [1, 2, 3]; i = myArray[0]; myArray[i]",
			2,
		},
		{
			"[1, 2, 3][3]",
			nil,
		},
		{
			"[1, 2, 3][-1]",
			3,
		},
		{
			"[1, 2, 3][-2]",
			2,
		},
		{
			"[1, 2, 3][-3]",
			1,
		},
		{
			"[1, 2, 3][-4]",
			nil,
		},
		{
			"[0, 1, 2, 3, 4, 5][2, 3]",
			[]int{2, 3, 4},
		},
		{
			"[0, 1, 2, 3, 4, 5][2..3]",
			[]int{2, 3},
		},
		{
			"[0, 1, 2, 3, 4, 5][2..-1]",
			[]int{2, 3, 4, 5},
		},
	}

	for _, tt := range tests {
		evaluated, err := testEval(tt.input)
		assert.NoError(t, err)
		switch expected := tt.expected.(type) {
		case int:
			testIntegerObject(t, evaluated, int64(expected))
		case []int:
			array, ok := evaluated.(*object.Array)
			assert.That(t, ok, "object is not Array. got=%T (%+v)", evaluated, evaluated)
			assert.Equal(t, len(array.Elements), len(expected))
			for i, v := range expected {
				testIntegerObject(t, array.Elements[i], int64(v))
			}
		case nil:
			testNilObject(t, evaluated)
		default:
			t.Logf("Expected %T, got %T\n", expected, evaluated)
		}
	}
}

func TestNilExpression(t *testing.T) {
	input := "nil"
	evaluated, err := testEval(input)
	assert.NoError(t, err)
	testNilObject(t, evaluated)
}

func TestHashLiteral(t *testing.T) {
	input := `{"foo" => 42, :bar => 2, true => false, nil => true, 2 => 2}`

	env := object.NewMainEnvironment()
	evaluated, err := testEval(input, env)
	assert.NoError(t, err)

	hash, ok := evaluated.(*object.Hash)
	assert.That(t, ok, "object is not Hash. got=%T (%+v)", evaluated, evaluated)

	expected := map[string]object.RubyObject{
		"foo":   object.NewInteger(42),
		":bar":  object.NewInteger(2),
		":nil":  object.TRUE,
		":true": object.FALSE,
		"2":     object.NewInteger(2),
	}

	actual := make(map[string]object.RubyObject)
	for k, v := range hash.ObjectMap() {
		actual[k.Inspect()] = v
	}

	assert.Equal(t, len(expected), len(actual))
	for k, v := range expected {
		actual_v, ok := actual[k]
		assert.That(t, ok, "Expected key %q to be present in hash", k)
		assert.EqualCmpAny(t, v, actual_v, object.CompareRubyObjectsForTests)
	}
}

func TestHashIndexExpressions(t *testing.T) {
	tests := []struct {
		input    string
		expected interface{}
	}{
		{
			"{'foo' => 1, 'bar' => 2, 'qux' => 3}['foo']",
			1,
		},
		{
			"{'foo' => 1, 'bar' => 2, 'qux' => 3}['bar']",
			2,
		},
		{
			"{'foo' => 1, 'bar' => 2, 'qux' => 3}['qux']",
			3,
		},
		{
			"i = 'foo'; {'foo'=>1}[i];",
			1,
		},
		{
			"{1=>1, 2=>2, 3=>3}[1 + 1];",
			2,
		},
		{
			"myHash = {1=>1, 2=>2, 3=>3}; myHash[2];",
			2,
		},
		{
			"myHash = {0=>1, 1=>2, 2=>3}; myHash[0] + myHash[1] + myHash[2];",
			6,
		},
		{
			"myHash = {0=>1, 1=>2, 2=>3}; i = myHash[0]; myHash[i]",
			2,
		},
		{
			"{0=>1, 1=>2, 2=>3}[3]",
			nil,
		},
		{
			"{0=>1, 1=>2, 2=>3}[-1]",
			nil,
		},
		{
			"{:foo => 1, :bar => 2, :qux => 3}[:qux]",
			3,
		},
		{
			"{'foo' =>1, true => 2, false => 3}[true]",
			2,
		},
		{
			"{nil =>1, :qux => 2, 3=>3}[nil]",
			1,
		},
	}

	for _, tt := range tests {
		evaluated, err := testEval(tt.input)
		assert.NoError(t, err)
		integer, ok := tt.expected.(int)
		if ok {
			testIntegerObject(t, evaluated, int64(integer))
		} else {
			testNilObject(t, evaluated)
		}
	}
}

func testNilObject(t *testing.T, obj object.RubyObject) bool {
	t.Helper()
	assert.EqualCmpAny(t, obj, object.NIL, object.CompareRubyObjectsForTests)
	return true
}

func testEval(input string, context ...object.Environment) (object.RubyObject, error) {
	env := object.NewEnvironment()
	if len(context) > 0 {
		env = context[0]
	}
	program, err := parser.ParseFile(token.NewFileSet(), "", input)
	if err != nil {
		return nil, object.NewSyntaxError(err)
	}
	return evaluator.Eval(program, env)
}

func testObject(t *testing.T, exp object.RubyObject, expected interface{}) {
	t.Helper()
	switch v := expected.(type) {
	case int:
		testIntegerObject(t, exp, int64(v))
	case int64:
		testIntegerObject(t, exp, v)
	case string:
		if strings.HasPrefix(v, ":") {
			testSymbolObject(t, exp, strings.TrimPrefix(v, ":"))
		}
		testStringObject(t, exp, v)
	case bool:
		testBooleanObject(t, exp, v)
	case map[string]string:
		testHashObject(t, exp, v)
	case []string:
		testArrayObject(t, exp, v)
	case nil:
		//
	default:
		t.Errorf("type of object not handled. got=%T", exp)
	}
}

func testBooleanObject(t *testing.T, obj object.RubyObject, expected bool) bool {
	t.Helper()
	result, ok := object.SymbolToBool(obj)
	assert.That(t, ok, "object is not Boolean. got=%T (%+v)", obj, obj)
	assert.Equal(t, result, expected)
	return true
}

func testIntegerObject(t *testing.T, obj object.RubyObject, expected int64) {
	t.Helper()
	result, ok := obj.(*object.Integer)
	assert.That(t, ok, "object is not Integer. got=%T (%+v)", obj, obj)
	assert.Equal(t, result.Value, expected)
}

func testSymbolObject(t *testing.T, obj object.RubyObject, expected string) {
	t.Helper()
	result, ok := obj.(*object.Symbol)
	assert.That(t, ok, "object is not Symbol. got=%T (%+v)", obj, obj)
	assert.Equal(t, result.Value, expected)
}

func testStringObject(t *testing.T, obj object.RubyObject, expected string) {
	t.Helper()
	result, ok := obj.(*object.String)
	assert.That(t, ok, "object is not String. got=%T (%+v)", obj, obj)
	assert.Equal(t, result.Value, expected)
}

func testHashObject(t *testing.T, obj object.RubyObject, expected map[string]string) {
	t.Helper()
	result, ok := obj.(*object.Hash)
	assert.That(t, ok, "object is not Hash. got=%T (%+v)", obj, obj)
	hashMap := make(map[string]string)
	for k, v := range result.ObjectMap() {
		hashMap[k.Inspect()] = v.Inspect()
	}
	assert.EqualCmp(t, hashMap, expected, compare.Maps)
}

func testArrayObject(t *testing.T, obj object.RubyObject, expected []string) {
	t.Helper()
	result, ok := obj.(*object.Array)
	assert.That(t, ok, "object is not Array. got=%T (%+v)", obj, obj)
	array := make([]string, len(result.Elements))
	for i, v := range result.Elements {
		array[i] = v.Inspect()
	}
	assert.EqualCmp(t, array, expected, compare.Arrays)
}
