package evaluator

import (
	"bufio"
	"io"
	"os"

	"github.com/lczyk/goruby/object"
)

// nativeFn is a generic body marker for Go-implemented methods.
// Lets us register builtins (File.read, STDIN.gets, ...) without
// growing the dispatchAttrMarker switch with a new typed marker per
// builtin.
type nativeFn struct {
	fn func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error)
}

// stdinReader returns a *bufio.Reader over env.Stdin(), cached on the
// root env so successive `gets` calls share buffered state and don't
// each read past their line.
func stdinReader(env *object.Environment) *bufio.Reader {
	if br, ok := env.StdinBR().(*bufio.Reader); ok && br != nil {
		return br
	}
	r := env.Stdin()
	if r == nil {
		r = os.Stdin
	}
	br := bufio.NewReader(r)
	env.SetStdinBR(br)
	return br
}

// bootstrapIO installs the File class and STDIN constant. Idempotent.
func bootstrapIO(env *object.Environment) {
	bootstrapFileClass(env)
	bootstrapSTDIN(env)
}

func bootstrapFileClass(env *object.Environment) *object.Class {
	if existing, ok := env.Get("File"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	c := object.NewClass("File", nil)
	c.ClassMethods["read"] = &object.UserMethod{Name: "read", Body: nativeFn{fn: fileRead}}
	c.ClassMethods["size"] = &object.UserMethod{Name: "size", Body: nativeFn{fn: fileSize}}
	c.ClassMethods["exist?"] = &object.UserMethod{Name: "exist?", Body: nativeFn{fn: fileExist}}
	c.ClassMethods["exists?"] = &object.UserMethod{Name: "exists?", Body: nativeFn{fn: fileExist}}
	env.SetGlobal("File", c)
	return c
}

func bootstrapSTDIN(env *object.Environment) *object.Class {
	if existing, ok := env.Get("STDIN"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	c := object.NewClass("STDIN", nil)
	c.ClassMethods["gets"] = &object.UserMethod{Name: "gets", Body: nativeFn{fn: stdinGets}}
	c.ClassMethods["read"] = &object.UserMethod{Name: "read", Body: nativeFn{fn: stdinReadAll}}
	c.ClassMethods["readline"] = &object.UserMethod{Name: "readline", Body: nativeFn{fn: stdinGets}}
	c.ClassMethods["eof?"] = &object.UserMethod{Name: "eof?", Body: nativeFn{fn: stdinEOF}}
	c.ClassMethods["eof"] = &object.UserMethod{Name: "eof", Body: nativeFn{fn: stdinEOF}}
	env.SetGlobal("STDIN", c)
	return c
}

func fileRead(_ *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, errorf("evaluator: File.read: wrong number of arguments (given %d, expected 1)", len(args))
	}
	path, ok := args[0].(*object.String)
	if !ok {
		return nil, errorf("evaluator: File.read: expected String path, got %T", args[0])
	}
	data, err := os.ReadFile(path.Value())
	if err != nil {
		return nil, errorf("evaluator: File.read: %s", err.Error())
	}
	return object.NewStringFromBytes(data), nil
}

func fileSize(_ *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, errorf("evaluator: File.size: wrong number of arguments (given %d, expected 1)", len(args))
	}
	path, ok := args[0].(*object.String)
	if !ok {
		return nil, errorf("evaluator: File.size: expected String path, got %T", args[0])
	}
	info, err := os.Stat(path.Value())
	if err != nil {
		return nil, errorf("evaluator: File.size: %s", err.Error())
	}
	return object.NewInteger(info.Size()), nil
}

func fileExist(_ *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, errorf("evaluator: File.exist?: wrong number of arguments (given %d, expected 1)", len(args))
	}
	path, ok := args[0].(*object.String)
	if !ok {
		return nil, errorf("evaluator: File.exist?: expected String path, got %T", args[0])
	}
	_, err := os.Stat(path.Value())
	if err == nil {
		return object.TRUE, nil
	}
	if os.IsNotExist(err) {
		return object.FALSE, nil
	}
	return nil, errorf("evaluator: File.exist?: %s", err.Error())
}

func stdinGets(env *object.Environment, _ []object.RubyObject) (object.RubyObject, error) {
	br := stdinReader(env)
	line, err := br.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, errorf("evaluator: gets: %s", err.Error())
	}
	if len(line) == 0 && err == io.EOF {
		return object.NIL, nil
	}
	return object.NewString(line), nil
}

func stdinReadAll(env *object.Environment, _ []object.RubyObject) (object.RubyObject, error) {
	br := stdinReader(env)
	data, err := io.ReadAll(br)
	if err != nil {
		return nil, errorf("evaluator: STDIN.read: %s", err.Error())
	}
	return object.NewStringFromBytes(data), nil
}

func stdinEOF(env *object.Environment, _ []object.RubyObject) (object.RubyObject, error) {
	br := stdinReader(env)
	if _, err := br.Peek(1); err == io.EOF {
		return object.TRUE, nil
	}
	return object.FALSE, nil
}
