package object

import (
	"testing"

	"github.com/MarcinKonowalczyk/goruby/trace"
	"github.com/lczyk/assert"
)

func TestWithArity(t *testing.T) {
	wrappedMethod := newMethod(func(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
		return NewInteger(1), nil
	})

	tests := []struct {
		arity     int
		arguments []RubyObject
		result    RubyObject
		err       error
	}{
		{
			1,
			[]RubyObject{NIL},
			NewInteger(1),
			nil,
		},
		{
			1,
			[]RubyObject{NIL, NIL},
			nil,
			NewWrongNumberOfArgumentsError(1, 2),
		},
		{
			2,
			[]RubyObject{NIL},
			nil,
			NewWrongNumberOfArgumentsError(2, 1),
		},
	}

	for _, testCase := range tests {
		fn := withArity(testCase.arity, wrappedMethod)
		context := &callContext{receiver: NIL}

		result, err := fn.Call(context, nil, testCase.arguments...)

		assert.EqualCmpAny(t, result, testCase.result, CompareRubyObjectsForTests)

		assert.Error(t, err, testCase.err)
	}
}
