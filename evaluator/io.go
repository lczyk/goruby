package evaluator

import (
	"bufio"
	"bytes"
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
	stdin := bootstrapSTDIN(env)
	stdout := bootstrapSTDOUT(env)
	stderr := bootstrapSTDERR(env)
	bootstrapARGF(env)
	// MRI exposes the three standard streams as `$stdin` / `$stdout` /
	// `$stderr` globals aliased to the STDIN / STDOUT / STDERR constants.
	// Stackcats (and most ruby code) routes IO through the globals so it
	// can be redirected; we mirror by binding the globals to the same
	// class objects on first bootstrap.
	if _, ok := env.Get("$stdin"); !ok {
		env.SetGlobal("$stdin", stdin)
	}
	if _, ok := env.Get("$stdout"); !ok {
		env.SetGlobal("$stdout", stdout)
	}
	// `$>` is an alias for $stdout (the default-output target used by
	// Kernel#print / Kernel#puts when no explicit IO is named). bind
	// to the same STDOUT class instance.
	if _, ok := env.Get("$>"); !ok {
		env.SetGlobal("$>", stdout)
	}
	// `$<` is an alias for ARGF (the default-input source). bind to
	// the same class object the ARGF bootstrap installs.
	if argf, ok := env.Get("ARGF"); ok {
		if _, ok := env.Get("$<"); !ok {
			env.SetGlobal("$<", argf)
		}
	}
	if _, ok := env.Get("$stderr"); !ok {
		env.SetGlobal("$stderr", stderr)
	}
	// `$/` is ruby's input record separator. Defaults to "\n"; very few
	// programs change it but several (stackcats among them) read it as
	// part of formatting. Bind once on bootstrap so a bare `$/` lookup
	// returns the newline rather than nil.
	if _, ok := env.Get("$/"); !ok {
		env.SetGlobal("$/", object.NewString("\n"))
	}
}

// bootstrapSTDOUT installs the STDOUT constant: a class object with
// `puts`/`print`/`write` class methods that delegate to env.Stdout().
// Idempotent.
func bootstrapSTDOUT(env *object.Environment) *object.Class {
	if existing, ok := env.Get("STDOUT"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	c := object.NewClass("STDOUT", nil)
	c.ClassMethods["puts"] = &object.UserMethod{Name: "puts", Body: nativeFn{fn: stdoutPuts}}
	c.ClassMethods["print"] = &object.UserMethod{Name: "print", Body: nativeFn{fn: stdoutPrint}}
	c.ClassMethods["write"] = &object.UserMethod{Name: "write", Body: nativeFn{fn: stdoutWrite}}
	c.ClassMethods["flush"] = &object.UserMethod{Name: "flush", Body: nativeFn{fn: ioNoopSelf}}
	c.ClassMethods["sync"] = &object.UserMethod{Name: "sync", Body: nativeFn{fn: ioReturnTrue}}
	c.ClassMethods["sync="] = &object.UserMethod{Name: "sync=", Body: nativeFn{fn: ioReturnArg}}
	c.ClassMethods["tty?"] = &object.UserMethod{Name: "tty?", Body: nativeFn{fn: stdoutTTY}}
	c.ClassMethods["binmode"] = &object.UserMethod{Name: "binmode", Body: nativeFn{fn: ioReturnSelf(c)}}
	c.ClassMethods["set_encoding"] = &object.UserMethod{Name: "set_encoding", Body: nativeFn{fn: ioReturnSelf(c)}}
	c.ClassMethods["putc"] = &object.UserMethod{Name: "putc", Body: nativeFn{fn: stdoutPutc}}
	env.SetGlobal("STDOUT", c)
	return c
}

func stdoutPutc(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	return kernelPutc(env, args)
}

// bootstrapSTDERR mirrors bootstrapSTDOUT but targets env.Stderr().
func bootstrapSTDERR(env *object.Environment) *object.Class {
	if existing, ok := env.Get("STDERR"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	c := object.NewClass("STDERR", nil)
	c.ClassMethods["puts"] = &object.UserMethod{Name: "puts", Body: nativeFn{fn: stderrPuts}}
	c.ClassMethods["print"] = &object.UserMethod{Name: "print", Body: nativeFn{fn: stderrPrint}}
	c.ClassMethods["write"] = &object.UserMethod{Name: "write", Body: nativeFn{fn: stderrWrite}}
	c.ClassMethods["flush"] = &object.UserMethod{Name: "flush", Body: nativeFn{fn: ioNoopSelf}}
	c.ClassMethods["sync"] = &object.UserMethod{Name: "sync", Body: nativeFn{fn: ioReturnTrue}}
	c.ClassMethods["sync="] = &object.UserMethod{Name: "sync=", Body: nativeFn{fn: ioReturnArg}}
	c.ClassMethods["binmode"] = &object.UserMethod{Name: "binmode", Body: nativeFn{fn: ioReturnSelf(c)}}
	c.ClassMethods["set_encoding"] = &object.UserMethod{Name: "set_encoding", Body: nativeFn{fn: ioReturnSelf(c)}}
	env.SetGlobal("STDERR", c)
	return c
}

func stdoutPuts(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	return kernelPuts(env, args)
}

func stdoutPrint(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	return kernelPrint(env, args)
}

func stdoutWrite(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	return ioWrite(env.Stdout(), env, args)
}

func stderrPuts(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	w := env.Stderr()
	if len(args) == 0 {
		_, _ = w.Write([]byte{'\n'})
		return object.NIL, nil
	}
	for _, a := range args {
		s := putsString(env, a)
		_, _ = w.Write([]byte(s))
		if len(s) == 0 || s[len(s)-1] != '\n' {
			_, _ = w.Write([]byte{'\n'})
		}
	}
	return object.NIL, nil
}

func stderrPrint(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	w := env.Stderr()
	for _, a := range args {
		_, _ = w.Write([]byte(putsString(env, a)))
	}
	return object.NIL, nil
}

func stderrWrite(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	return ioWrite(env.Stderr(), env, args)
}

// ioWrite implements IO#write: writes each arg's to_s, returns the
// total byte count written.
func ioWrite(w io.Writer, env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	total := 0
	for _, a := range args {
		s := putsString(env, a)
		n, _ := w.Write([]byte(s))
		total += n
	}
	return object.NewInteger(int64(total)), nil
}

func stdoutTTY(env *object.Environment, _ []object.RubyObject) (object.RubyObject, error) {
	f, ok := env.Stdout().(*os.File)
	if !ok {
		return object.FALSE, nil
	}
	info, err := f.Stat()
	if err != nil {
		return object.FALSE, nil
	}
	if info.Mode()&os.ModeCharDevice != 0 {
		return object.TRUE, nil
	}
	return object.FALSE, nil
}

func ioNoopSelf(_ *object.Environment, _ []object.RubyObject) (object.RubyObject, error) {
	return object.NIL, nil
}

func ioReturnTrue(_ *object.Environment, _ []object.RubyObject) (object.RubyObject, error) {
	return object.TRUE, nil
}

// ioReturnSelf builds a no-op method that returns the bound class.
// Used for ruby IO methods we don't actually implement (binmode,
// set_encoding, etc.) but that callers chain off of -- the receiver
// must come back so subsequent calls in the chain still work.
func ioReturnSelf(c *object.Class) func(*object.Environment, []object.RubyObject) (object.RubyObject, error) {
	return func(_ *object.Environment, _ []object.RubyObject) (object.RubyObject, error) {
		return c, nil
	}
}

func ioReturnArg(_ *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) >= 1 {
		return args[0], nil
	}
	return object.NIL, nil
}

func bootstrapARGF(env *object.Environment) *object.Class {
	if existing, ok := env.Get("ARGF"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	c := object.NewClass("ARGF", nil)
	c.ClassMethods["read"] = &object.UserMethod{Name: "read", Body: nativeFn{fn: argfRead}}
	c.ClassMethods["gets"] = &object.UserMethod{Name: "gets", Body: nativeFn{fn: argfGets}}
	c.ClassMethods["readline"] = &object.UserMethod{Name: "readline", Body: nativeFn{fn: argfGets}}
	c.ClassMethods["each_line"] = &object.UserMethod{Name: "each_line", Body: nativeFn{fn: argfEachLine}}
	env.SetGlobal("ARGF", c)
	return c
}

// argfReader returns a *bufio.Reader covering the concatenation of all
// ARGV files (or stdin if ARGV is empty), cached on the env so successive
// ARGF.gets calls advance through the same stream. Mirrors MRI's
// ARGF semantics enough for typical "first line is source" CLI scripts.
func argfReader(env *object.Environment) (*bufio.Reader, error) {
	if br, ok := env.ArgfBR().(*bufio.Reader); ok && br != nil {
		return br, nil
	}
	argv, _ := env.Get("ARGV")
	arr, _ := argv.(*object.Array)
	if arr == nil || len(arr.Elements) == 0 {
		br := stdinReader(env)
		env.SetArgfBR(br)
		return br, nil
	}
	var buf []byte
	for _, e := range arr.Elements {
		s, ok := e.(*object.String)
		if !ok {
			return nil, errorf("evaluator: ARGF: ARGV element not String: %T", e)
		}
		data, err := os.ReadFile(s.Value())
		if err != nil {
			return nil, errorf("evaluator: ARGF: %s", err.Error())
		}
		buf = append(buf, data...)
	}
	br := bufio.NewReader(bytes.NewReader(buf))
	env.SetArgfBR(br)
	return br, nil
}

// argfGets reads one line (incl. terminator) from the ARGF stream;
// returns nil at EOF. Matches Kernel#gets for the common one-line
// case CLI scripts use.
func argfGets(env *object.Environment, _ []object.RubyObject) (object.RubyObject, error) {
	br, err := argfReader(env)
	if err != nil {
		return nil, err
	}
	line, err := br.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, errorf("evaluator: ARGF.gets: %s", err.Error())
	}
	if len(line) == 0 && err == io.EOF {
		return object.NIL, nil
	}
	return object.NewString(line), nil
}

func argfEachLine(env *object.Environment, _ []object.RubyObject) (object.RubyObject, error) {
	// Eagerly slurp the ARGF stream into lines and wrap in an Enumerator.
	// We don't have a lazy Enumerator yet; the slurp is fine for the
	// CLI patterns the corpus uses (file/stdin -> line iteration).
	var lines []object.RubyObject
	for {
		v, err := argfGets(env, nil)
		if err != nil {
			return nil, err
		}
		if _, isNil := v.(*object.Nil); isNil {
			break
		}
		lines = append(lines, v)
	}
	return &object.Enumerator{Receiver: object.NewArray(lines...), Method: "each"}, nil
}

// argfRead reads the concatenation of every file named in ARGV; if
// ARGV is empty or unset, falls back to slurping stdin. Mirrors MRI's
// ARGF.read.
func argfRead(env *object.Environment, _ []object.RubyObject) (object.RubyObject, error) {
	if argv, ok := env.Get("ARGV"); ok {
		if arr, ok := argv.(*object.Array); ok && len(arr.Elements) > 0 {
			var buf []byte
			for _, e := range arr.Elements {
				s, ok := e.(*object.String)
				if !ok {
					return nil, errorf("evaluator: ARGF.read: ARGV element not String: %T", e)
				}
				data, err := os.ReadFile(s.Value())
				if err != nil {
					return nil, errorf("evaluator: ARGF.read: %s", err.Error())
				}
				buf = append(buf, data...)
			}
			return object.NewStringFromBytes(buf), nil
		}
	}
	return stdinReadAll(env, nil)
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
	c.ClassMethods["getbyte"] = &object.UserMethod{Name: "getbyte", Body: nativeFn{fn: stdinGetbyte}}
	c.ClassMethods["getc"] = &object.UserMethod{Name: "getc", Body: nativeFn{fn: stdinGetc}}
	c.ClassMethods["tty?"] = &object.UserMethod{Name: "tty?", Body: nativeFn{fn: stdinTTY}}
	c.ClassMethods["isatty"] = &object.UserMethod{Name: "isatty", Body: nativeFn{fn: stdinTTY}}
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

func stdinReadAll(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	br := stdinReader(env)
	// IO#read(n) returns up to n bytes, or nil at EOF when n>0. Without
	// n it slurps the rest of the stream and returns "" at EOF.
	if len(args) >= 1 {
		n, ok := args[0].(*object.Integer)
		if !ok {
			return nil, errorf("evaluator: STDIN.read: expected Integer length, got %T", args[0])
		}
		if n.Value < 0 {
			return nil, errorf("evaluator: STDIN.read: negative length %d", n.Value)
		}
		if n.Value == 0 {
			return object.NewString(""), nil
		}
		buf := make([]byte, n.Value)
		got, err := io.ReadFull(br, buf)
		if got == 0 {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return object.NIL, nil
			}
			if err != nil {
				return nil, errorf("evaluator: STDIN.read: %s", err.Error())
			}
		}
		return object.NewStringFromBytes(buf[:got]), nil
	}
	data, err := io.ReadAll(br)
	if err != nil {
		return nil, errorf("evaluator: STDIN.read: %s", err.Error())
	}
	return object.NewStringFromBytes(data), nil
}

func stdinGetbyte(env *object.Environment, _ []object.RubyObject) (object.RubyObject, error) {
	br := stdinReader(env)
	b, err := br.ReadByte()
	if err == io.EOF {
		return object.NIL, nil
	}
	if err != nil {
		return nil, errorf("evaluator: STDIN.getbyte: %s", err.Error())
	}
	return object.NewInteger(int64(b)), nil
}

func stdinGetc(env *object.Environment, _ []object.RubyObject) (object.RubyObject, error) {
	br := stdinReader(env)
	r, _, err := br.ReadRune()
	if err == io.EOF {
		return object.NIL, nil
	}
	if err != nil {
		return nil, errorf("evaluator: STDIN.getc: %s", err.Error())
	}
	return object.NewString(string(r)), nil
}

// stdinTTY implements STDIN.tty? / STDIN.isatty: reports whether the
// active stdin is a character device. Only true when env.Stdin() is
// the real os.Stdin AND that file's stat indicates a tty. Test harnesses
// that pipe via WithStdin always see false.
func stdinTTY(env *object.Environment, _ []object.RubyObject) (object.RubyObject, error) {
	f, ok := env.Stdin().(*os.File)
	if !ok {
		return object.FALSE, nil
	}
	info, err := f.Stat()
	if err != nil {
		return object.FALSE, nil
	}
	if info.Mode()&os.ModeCharDevice != 0 {
		return object.TRUE, nil
	}
	return object.FALSE, nil
}

func stdinEOF(env *object.Environment, _ []object.RubyObject) (object.RubyObject, error) {
	br := stdinReader(env)
	if _, err := br.Peek(1); err == io.EOF {
		return object.TRUE, nil
	}
	return object.FALSE, nil
}
