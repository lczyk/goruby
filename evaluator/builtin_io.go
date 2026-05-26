package evaluator

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lczyk/goruby/evaluator/builtinapi"
	"github.com/lczyk/goruby/object"
)

// nativeFn is a generic body marker for Go-implemented methods.
// Lets us register builtins (File.read, STDIN.gets, ...) without
// growing the dispatchAttrMarker switch with a new typed marker per
// builtin.
// nativeFn aliases builtinapi.NativeFn so existing call sites keep
// their lowercase identifier; the underlying type lives in builtinapi
// so subpackages can construct it.
type nativeFn = builtinapi.NativeFn

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
	bootstrapSignalClass(env)
	bootstrapIOClass(env)
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

// bootstrapIOClass installs the IO class. MRI's STDOUT and STDERR
// are IO instances; in goruby they're Class objects (legacy choice),
// so we install IO with the same interface stubs and make
// STDOUT.Super / STDERR.Super = IO so .is_a?(IO) at the class-as-
// receiver level evaluates true via the ancestor walk. Class
// methods (puts, print, write, etc.) are inherited automatically.
func bootstrapIOClass(env *object.Environment) *object.Class {
	if existing, ok := env.Get("IO"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	c := object.NewClass("IO", nil)
	env.SetGlobal("IO", c)
	return c
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
	io, _ := env.Get("IO")
	ioCls, _ := io.(*object.Class)
	c := object.NewClass("STDOUT", ioCls)
	c.ClassMethods["puts"] = &object.UserMethod{Name: "puts", Body: nativeFn{Fn: stdoutPuts}}
	c.ClassMethods["print"] = &object.UserMethod{Name: "print", Body: nativeFn{Fn: stdoutPrint}}
	c.ClassMethods["write"] = &object.UserMethod{Name: "write", Body: nativeFn{Fn: stdoutWrite}}
	c.ClassMethods["flush"] = &object.UserMethod{Name: "flush", Body: nativeFn{Fn: ioNoopSelf}}
	c.ClassMethods["sync"] = &object.UserMethod{Name: "sync", Body: nativeFn{Fn: ioReturnTrue}}
	c.ClassMethods["sync="] = &object.UserMethod{Name: "sync=", Body: nativeFn{Fn: ioReturnArg}}
	c.ClassMethods["tty?"] = &object.UserMethod{Name: "tty?", Body: nativeFn{Fn: stdoutTTY}}
	c.ClassMethods["binmode"] = &object.UserMethod{Name: "binmode", Body: nativeFn{Fn: ioReturnSelf(c)}}
	c.ClassMethods["set_encoding"] = &object.UserMethod{Name: "set_encoding", Body: nativeFn{Fn: ioReturnSelf(c)}}
	c.ClassMethods["putc"] = &object.UserMethod{Name: "putc", Body: nativeFn{Fn: stdoutPutc}}
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
	io, _ := env.Get("IO")
	ioCls, _ := io.(*object.Class)
	c := object.NewClass("STDERR", ioCls)
	c.ClassMethods["puts"] = &object.UserMethod{Name: "puts", Body: nativeFn{Fn: stderrPuts}}
	c.ClassMethods["print"] = &object.UserMethod{Name: "print", Body: nativeFn{Fn: stderrPrint}}
	c.ClassMethods["write"] = &object.UserMethod{Name: "write", Body: nativeFn{Fn: stderrWrite}}
	c.ClassMethods["flush"] = &object.UserMethod{Name: "flush", Body: nativeFn{Fn: ioNoopSelf}}
	c.ClassMethods["sync"] = &object.UserMethod{Name: "sync", Body: nativeFn{Fn: ioReturnTrue}}
	c.ClassMethods["sync="] = &object.UserMethod{Name: "sync=", Body: nativeFn{Fn: ioReturnArg}}
	c.ClassMethods["binmode"] = &object.UserMethod{Name: "binmode", Body: nativeFn{Fn: ioReturnSelf(c)}}
	c.ClassMethods["set_encoding"] = &object.UserMethod{Name: "set_encoding", Body: nativeFn{Fn: ioReturnSelf(c)}}
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
	c.ClassMethods["read"] = &object.UserMethod{Name: "read", Body: nativeFn{Fn: argfRead}}
	c.ClassMethods["gets"] = &object.UserMethod{Name: "gets", Body: nativeFn{Fn: argfGets}}
	c.ClassMethods["readline"] = &object.UserMethod{Name: "readline", Body: nativeFn{Fn: argfGets}}
	c.ClassMethods["each_line"] = &object.UserMethod{Name: "each_line", Body: nativeFn{Fn: argfEachLine}}
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

// bootstrapSignalClass installs a minimal Signal stub. Scripts use
// Signal.trap(:INT) { ... } to install Ctrl-C handlers; under test
// runs we accept the registration and discard it.
func bootstrapSignalClass(env *object.Environment) *object.Class {
	if existing, ok := env.Get("Signal"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	c := object.NewClass("Signal", nil)
	c.ClassMethods["trap"] = &object.UserMethod{Name: "trap", Body: nativeFn{Fn: func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		// Stub: don't install OS-level handlers. Discard any block via
		// the dispatcher-stashed CurrentBlock and return nil.
		return object.NIL, nil
	}}}
	c.ClassMethods["list"] = &object.UserMethod{Name: "list", Body: nativeFn{Fn: func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		return object.NewHash(), nil
	}}}
	env.SetGlobal("Signal", c)
	return c
}

func bootstrapFileClass(env *object.Environment) *object.Class {
	if existing, ok := env.Get("File"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	c := object.NewClass("File", nil)
	c.ClassMethods["read"] = &object.UserMethod{Name: "read", Body: nativeFn{Fn: fileRead}}
	c.ClassMethods["size"] = &object.UserMethod{Name: "size", Body: nativeFn{Fn: fileSize}}
	c.ClassMethods["exist?"] = &object.UserMethod{Name: "exist?", Body: nativeFn{Fn: fileExist}}
	c.ClassMethods["exists?"] = &object.UserMethod{Name: "exists?", Body: nativeFn{Fn: fileExist}}
	c.ClassMethods["new"] = &object.UserMethod{Name: "new", Body: nativeFn{Fn: fileNew(c)}}
	c.ClassMethods["open"] = &object.UserMethod{Name: "open", Body: nativeFn{Fn: fileNew(c)}}
	c.ClassMethods["join"] = &object.UserMethod{Name: "join", Body: nativeFn{Fn: fileJoin}}
	c.ClassMethods["dirname"] = &object.UserMethod{Name: "dirname", Body: nativeFn{Fn: fileDirname}}
	c.ClassMethods["split"] = &object.UserMethod{Name: "split", Body: nativeFn{Fn: fileSplit}}
	// MRI File::SEPARATOR ("/"") and File::ALT_SEPARATOR (nil on
	// posix, "\\" on win). Rake's pathmap "%s" reads them; without
	// the constants the chain blows up before the format walker.
	if c.Constants == nil {
		c.Constants = map[string]object.RubyObject{}
	}
	c.Constants["SEPARATOR"] = object.NewString("/")
	c.Constants["Separator"] = c.Constants["SEPARATOR"]
	c.Constants["PATH_SEPARATOR"] = object.NewString(":")
	c.Constants["ALT_SEPARATOR"] = object.NIL
	// FNM_* glob match flags. MRI exposes these as Integer bitmasks
	// on File. Numeric values match MRI's fnmatch implementation.
	c.Constants["FNM_NOESCAPE"] = object.NewInteger(0x01)
	c.Constants["FNM_PATHNAME"] = object.NewInteger(0x02)
	c.Constants["FNM_DOTMATCH"] = object.NewInteger(0x04)
	c.Constants["FNM_CASEFOLD"] = object.NewInteger(0x08)
	c.Constants["FNM_EXTGLOB"] = object.NewInteger(0x10)
	c.Constants["FNM_SYSCASE"] = object.NewInteger(0x00)
	c.Constants["FNM_SHORTNAME"] = object.NewInteger(0x40)
	c.ClassMethods["basename"] = &object.UserMethod{Name: "basename", Body: nativeFn{Fn: fileBasename}}
	c.ClassMethods["extname"] = &object.UserMethod{Name: "extname", Body: nativeFn{Fn: fileExtname}}
	c.ClassMethods["expand_path"] = &object.UserMethod{Name: "expand_path", Body: nativeFn{Fn: fileExpandPath}}
	c.ClassMethods["realpath"] = &object.UserMethod{Name: "realpath", Body: nativeFn{Fn: fileExpandPath}}
	c.ClassMethods["absolute_path"] = &object.UserMethod{Name: "absolute_path", Body: nativeFn{Fn: fileExpandPath}}
	c.ClassMethods["write"] = &object.UserMethod{Name: "write", Body: nativeFn{Fn: fileWrite}}
	c.ClassMethods["mtime"] = &object.UserMethod{Name: "mtime", Body: nativeFn{Fn: fileMtime}}
	c.ClassMethods["stat"] = &object.UserMethod{Name: "stat", Body: nativeFn{Fn: fileStat(c)}}
	c.ClassMethods["utime"] = &object.UserMethod{Name: "utime", Body: nativeFn{Fn: fileUtime}}
	c.ClassMethods["chmod"] = &object.UserMethod{Name: "chmod", Body: nativeFn{Fn: fileChmod}}
	c.ClassMethods["directory?"] = &object.UserMethod{Name: "directory?", Body: nativeFn{Fn: fileDirectoryQ}}
	c.ClassMethods["file?"] = &object.UserMethod{Name: "file?", Body: nativeFn{Fn: fileFileQ}}
	c.ClassMethods["readable?"] = &object.UserMethod{Name: "readable?", Body: nativeFn{Fn: fileReadableQ}}
	c.ClassMethods["writable?"] = &object.UserMethod{Name: "writable?", Body: nativeFn{Fn: fileWritableQ}}
	c.ClassMethods["executable?"] = &object.UserMethod{Name: "executable?", Body: nativeFn{Fn: fileExecutableQ}}
	c.Methods["each"] = &object.BuiltinMethod{Name: "each", Fn: fileEach}
	c.Methods["each_line"] = c.Methods["each"]
	c.Methods["read"] = &object.BuiltinMethod{Name: "read", Fn: fileInstanceRead}
	c.Methods["close"] = &object.BuiltinMethod{Name: "close", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return object.NIL, nil
	}}
	env.SetGlobal("File", c)
	return c
}

// fileNew eagerly slurps the file into @lines so the resulting Instance
// supports each / each_line without holding an open OS handle. Mirrors
// MRI's File.new(path, mode) closely enough for read-mostly corpus use;
// write modes are accepted but ignored.
func fileNew(c *object.Class) func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	return func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) < 1 {
			return nil, errorf("evaluator: File.new: missing path")
		}
		path, ok := stringText(env, args[0])
		if !ok {
			return nil, errorf("evaluator: File.new: expected String path, got %T", args[0])
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return raiseBuiltin(env, "Errno::ENOENT", err.Error())
		}
		text := string(data)
		lines := strings.SplitAfter(text, "\n")
		// SplitAfter leaves a trailing empty string when the file ends
		// w/ a newline; drop it so iteration count matches MRI.
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		lineObjs := make([]object.RubyObject, len(lines))
		for i, l := range lines {
			lineObjs[i] = object.NewString(l)
		}
		return &object.Instance{
			C: c,
			Ivars: map[string]object.RubyObject{
				"@path":  object.NewString(path),
				"@text":  object.NewString(text),
				"@lines": object.NewArray(lineObjs...),
			},
		}, nil
	}
}

func fileEach(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	inst, ok := recv.(*object.Instance)
	if !ok {
		return nil, errorf("evaluator: File#each on non-File %T", recv)
	}
	lines, ok := inst.Ivars["@lines"].(*object.Array)
	if !ok {
		return recv, nil
	}
	for _, line := range lines.Elements {
		if _, err := builtinapi.InvokeBlockValue(env, block, []object.RubyObject{line}); err != nil {
			return nil, err
		}
	}
	return recv, nil
}

func fileInstanceRead(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	inst, ok := recv.(*object.Instance)
	if !ok {
		return nil, errorf("evaluator: File#read on non-File %T", recv)
	}
	if s, ok := inst.Ivars["@text"].(*object.String); ok {
		return s, nil
	}
	return object.NewString(""), nil
}

func bootstrapSTDIN(env *object.Environment) *object.Class {
	if existing, ok := env.Get("STDIN"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	c := object.NewClass("STDIN", nil)
	c.ClassMethods["gets"] = &object.UserMethod{Name: "gets", Body: nativeFn{Fn: stdinGets}}
	c.ClassMethods["read"] = &object.UserMethod{Name: "read", Body: nativeFn{Fn: stdinReadAll}}
	c.ClassMethods["readline"] = &object.UserMethod{Name: "readline", Body: nativeFn{Fn: stdinGets}}
	c.ClassMethods["eof?"] = &object.UserMethod{Name: "eof?", Body: nativeFn{Fn: stdinEOF}}
	c.ClassMethods["eof"] = &object.UserMethod{Name: "eof", Body: nativeFn{Fn: stdinEOF}}
	c.ClassMethods["getbyte"] = &object.UserMethod{Name: "getbyte", Body: nativeFn{Fn: stdinGetbyte}}
	c.ClassMethods["getc"] = &object.UserMethod{Name: "getc", Body: nativeFn{Fn: stdinGetc}}
	c.ClassMethods["tty?"] = &object.UserMethod{Name: "tty?", Body: nativeFn{Fn: stdinTTY}}
	c.ClassMethods["isatty"] = &object.UserMethod{Name: "isatty", Body: nativeFn{Fn: stdinTTY}}
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

// fileJoin: File.join(seg, seg, ...) -> "seg/seg/...". Mirrors MRI:
// empty segments collapse, redundant separators around each join site
// dedupe. Array args are recursively joined too (so File.join(["a","b"],
// "c") -> "a/b/c").
func fileJoin(_ *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	parts := make([]string, 0, len(args))
	var collect func(o object.RubyObject) error
	collect = func(o object.RubyObject) error {
		switch v := o.(type) {
		case *object.String:
			parts = append(parts, v.Value())
		case *object.Array:
			for _, e := range v.Elements {
				if err := collect(e); err != nil {
					return err
				}
			}
		default:
			return errorf("evaluator: File.join: expected String/Array, got %T", o)
		}
		return nil
	}
	for _, a := range args {
		if err := collect(a); err != nil {
			return nil, err
		}
	}
	// Trim leading/trailing separator from each interior piece so the
	// canonical form has exactly one separator between segments. MRI
	// preserves an absolute-style leading separator on the first piece.
	for i := range parts {
		if i > 0 {
			parts[i] = strings.TrimLeft(parts[i], "/")
		}
		if i < len(parts)-1 {
			parts[i] = strings.TrimRight(parts[i], "/")
		}
	}
	return object.NewString(strings.Join(parts, "/")), nil
}

// fileSplit mirrors MRI's File.split -- returns [dirname, basename] as
// a two-element Array. Rake's pathmap_explode iterates via this.
func fileSplit(_ *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, errorf("evaluator: File.split: expected 1 arg, got %d", len(args))
	}
	s, ok := args[0].(*object.String)
	if !ok {
		return nil, errorf("evaluator: File.split: expected String, got %T", args[0])
	}
	v := s.Value()
	return object.NewArray(
		object.NewString(filepath.Dir(v)),
		object.NewString(filepath.Base(v)),
	), nil
}

func fileDirname(_ *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, errorf("evaluator: File.dirname: expected 1 arg, got %d", len(args))
	}
	s, ok := args[0].(*object.String)
	if !ok {
		return nil, errorf("evaluator: File.dirname: expected String, got %T", args[0])
	}
	return object.NewString(filepath.Dir(s.Value())), nil
}

func fileBasename(_ *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, errorf("evaluator: File.basename: expected 1..2 args, got %d", len(args))
	}
	s, ok := args[0].(*object.String)
	if !ok {
		return nil, errorf("evaluator: File.basename: expected String, got %T", args[0])
	}
	base := filepath.Base(s.Value())
	if len(args) == 2 {
		ext, ok := args[1].(*object.String)
		if !ok {
			return nil, errorf("evaluator: File.basename: expected String ext, got %T", args[1])
		}
		extVal := ext.Value()
		if extVal == ".*" {
			if dot := strings.LastIndexByte(base, '.'); dot > 0 {
				base = base[:dot]
			}
		} else if strings.HasSuffix(base, extVal) {
			base = base[:len(base)-len(extVal)]
		}
	}
	return object.NewString(base), nil
}

func fileExtname(_ *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, errorf("evaluator: File.extname: expected 1 arg, got %d", len(args))
	}
	s, ok := args[0].(*object.String)
	if !ok {
		return nil, errorf("evaluator: File.extname: expected String, got %T", args[0])
	}
	// MRI's File.extname returns "" for dotfiles (no dot-prefix
	// counts as the extension separator). Compare basename's
	// rightmost dot position: if there is none, or it's at index 0,
	// no extension.
	base := filepath.Base(s.Value())
	dot := strings.LastIndexByte(base, '.')
	if dot <= 0 {
		return object.NewString(""), nil
	}
	return object.NewString(base[dot:]), nil
}

func fileExpandPath(_ *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, errorf("evaluator: File.expand_path: expected 1..2 args, got %d", len(args))
	}
	s, ok := args[0].(*object.String)
	if !ok {
		return nil, errorf("evaluator: File.expand_path: expected String, got %T", args[0])
	}
	target := s.Value()
	// MRI expands ~ to $HOME. Simplistic handling; enough for the
	// most common cases callers reach for.
	if strings.HasPrefix(target, "~/") || target == "~" {
		if home, _ := os.UserHomeDir(); home != "" {
			target = filepath.Join(home, strings.TrimPrefix(target, "~"))
		}
	}
	if !filepath.IsAbs(target) {
		base := ""
		if len(args) == 2 {
			b, ok := args[1].(*object.String)
			if !ok {
				return nil, errorf("evaluator: File.expand_path: expected String base, got %T", args[1])
			}
			base = b.Value()
		}
		if base == "" {
			cwd, err := os.Getwd()
			if err != nil {
				return nil, errorf("evaluator: File.expand_path: %s", err.Error())
			}
			base = cwd
		}
		target = filepath.Join(base, target)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return nil, errorf("evaluator: File.expand_path: %s", err.Error())
	}
	return object.NewString(abs), nil
}

func fileWrite(_ *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) < 2 {
		return nil, errorf("evaluator: File.write: expected 2..3 args, got %d", len(args))
	}
	path, ok := args[0].(*object.String)
	if !ok {
		return nil, errorf("evaluator: File.write: expected String path, got %T", args[0])
	}
	var data []byte
	switch s := args[1].(type) {
	case *object.String:
		data = []byte(s.Value())
	default:
		return nil, errorf("evaluator: File.write: expected String content, got %T", args[1])
	}
	if err := os.WriteFile(path.Value(), data, 0o644); err != nil {
		return nil, errorf("evaluator: File.write: %s", err.Error())
	}
	return object.NewInteger(int64(len(data))), nil
}

// fileChmod mirrors MRI's File.chmod(mode, *paths) -- changes the
// Unix permission bits on each path. Returns the count of paths
// changed.
func fileChmod(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) < 2 {
		return nil, errorf("evaluator: File.chmod: expected mode + at least one path")
	}
	modeInt, ok := args[0].(*object.Integer)
	if !ok {
		return nil, errorf("evaluator: File.chmod: expected Integer mode, got %T", args[0])
	}
	count := 0
	for _, p := range args[1:] {
		s, ok := stringText(env, p)
		if !ok {
			return nil, errorf("evaluator: File.chmod: expected String path, got %T", p)
		}
		if err := os.Chmod(s, os.FileMode(modeInt.Value)); err != nil {
			return raiseBuiltin(env, "Errno::ENOENT", err.Error())
		}
		count++
	}
	return object.NewInteger(int64(count)), nil
}

// fileUtime mirrors MRI's File.utime(atime, mtime, *paths) -- sets
// access + modify times on each path. Returns the number of paths
// touched. Rake's file_creation helper bumps mtime to forge "old" /
// "new" timestamps in tests.
func fileUtime(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) < 3 {
		return nil, errorf("evaluator: File.utime: expected atime, mtime, *paths")
	}
	atime, aErr := timeFromRubyObject(args[0])
	if aErr != nil {
		return nil, aErr
	}
	mtime, mErr := timeFromRubyObject(args[1])
	if mErr != nil {
		return nil, mErr
	}
	count := 0
	for _, p := range args[2:] {
		s, ok := stringText(env, p)
		if !ok {
			return nil, errorf("evaluator: File.utime: expected String path, got %T", p)
		}
		if err := os.Chtimes(s, atime, mtime); err != nil {
			return raiseBuiltin(env, "Errno::ENOENT", err.Error())
		}
		count++
	}
	return object.NewInteger(int64(count)), nil
}

// timeFromRubyObject extracts a time.Time from either a Time Instance
// (@__unix__ / @__nsec__ ivars laid down by bootstrapTimeClass) or
// a numeric Integer / Float epoch seconds.
func timeFromRubyObject(o object.RubyObject) (time.Time, error) {
	switch v := o.(type) {
	case *object.Integer:
		return time.Unix(v.Value, 0), nil
	case *object.Float:
		sec := int64(v.Value)
		nsec := int64((v.Value - float64(sec)) * 1e9)
		return time.Unix(sec, nsec), nil
	case *object.Instance:
		var sec, nsec int64
		if u, ok := v.Ivars["@__unix__"].(*object.Integer); ok {
			sec = u.Value
		}
		if n, ok := v.Ivars["@__nsec__"].(*object.Integer); ok {
			nsec = n.Value
		}
		return time.Unix(sec, nsec), nil
	}
	return time.Time{}, errorf("evaluator: File.utime: expected Time / Numeric, got %T", o)
}

// fileStat returns an Instance with read-accessor ivars for the common
// stat fields that rake/minitest poke at: mtime (Time), size (Integer),
// directory? / file? predicates. Closure captures the File class so
// the Stat shares ancestors with File for ===/is_a? if anyone asks
// (no real File::Stat class to mirror, alas).
func fileStat(fileClass *object.Class) func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	return func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: File.stat: expected 1 arg, got %d", len(args))
		}
		path, ok := args[0].(*object.String)
		if !ok {
			return nil, errorf("evaluator: File.stat: expected String, got %T", args[0])
		}
		info, err := os.Stat(path.Value())
		if err != nil {
			return raiseBuiltin(env, "Errno::ENOENT", err.Error())
		}
		mtime, mErr := fileMtime(env, []object.RubyObject{path})
		if mErr != nil {
			return nil, mErr
		}
		statCls, ok := fileClass.Constants["Stat"].(*object.Class)
		if !ok {
			statCls = object.NewClass("Stat", nil)
			readIvar := func(name string) *object.BuiltinMethod {
				return &object.BuiltinMethod{Name: name, Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
					inst, _ := recv.(*object.Instance)
					if v, ok := inst.Ivars["@"+name]; ok {
						return v, nil
					}
					return object.NIL, nil
				}}
			}
			statCls.AddMethod("mtime", readIvar("mtime"))
			statCls.AddMethod("size", readIvar("size"))
			statCls.AddMethod("directory?", readIvar("directory?"))
			statCls.AddMethod("file?", readIvar("file?"))
			if fileClass.Constants == nil {
				fileClass.Constants = map[string]object.RubyObject{}
			}
			fileClass.Constants["Stat"] = statCls
		}
		inst := object.NewInstance(statCls)
		inst.Ivars["@mtime"] = mtime
		inst.Ivars["@size"] = object.NewInteger(info.Size())
		inst.Ivars["@directory?"] = object.BooleanOf(info.IsDir())
		inst.Ivars["@file?"] = object.BooleanOf(info.Mode().IsRegular())
		return inst, nil
	}
}

func fileMtime(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, errorf("evaluator: File.mtime: expected 1 arg, got %d", len(args))
	}
	path, ok := args[0].(*object.String)
	if !ok {
		return nil, errorf("evaluator: File.mtime: expected String, got %T", args[0])
	}
	info, err := os.Stat(path.Value())
	if err != nil {
		// MRI raises Errno::ENOENT for missing files. rake's
		// FileTask#needed? hinges on this -- it treats the raised
		// exception as "target doesn't exist, must rebuild".
		return raiseBuiltin(env, "Errno::ENOENT", err.Error())
	}
	// Return a Time instance (with @__unix__ / @__nsec__ ivars) so
	// callers can compare via <=>, format via strftime, etc. Bootstrap
	// the Time class lazily in case env didn't include it; safe even
	// if called concurrently since bootstrapTimeClass is idempotent.
	timeCls, ok := env.Get("Time")
	if !ok {
		return object.NewInteger(info.ModTime().Unix()), nil
	}
	tc, ok := timeCls.(*object.Class)
	if !ok {
		return object.NewInteger(info.ModTime().Unix()), nil
	}
	inst := object.NewInstance(tc)
	inst.Ivars["@__unix__"] = object.NewInteger(info.ModTime().Unix())
	inst.Ivars["@__nsec__"] = object.NewInteger(int64(info.ModTime().Nanosecond()))
	return inst, nil
}

func fileDirectoryQ(_ *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, errorf("evaluator: File.directory?: expected 1 arg, got %d", len(args))
	}
	path, ok := args[0].(*object.String)
	if !ok {
		return nil, errorf("evaluator: File.directory?: expected String, got %T", args[0])
	}
	info, err := os.Stat(path.Value())
	if err != nil {
		return object.FALSE, nil
	}
	return object.BooleanOf(info.IsDir()), nil
}

// fileReadableQ / fileWritableQ / fileExecutableQ test the
// corresponding Unix permission bits via os.Stat. Rake's
// Cleaner.cant_be_deleted? probes these on a path before deleting.
func fileReadableQ(_ *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	mode, ok := statModeOf(args)
	if !ok {
		return object.FALSE, nil
	}
	return object.BooleanOf(mode&0o400 != 0), nil
}

func fileWritableQ(_ *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	mode, ok := statModeOf(args)
	if !ok {
		return object.FALSE, nil
	}
	return object.BooleanOf(mode&0o200 != 0), nil
}

func fileExecutableQ(_ *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	mode, ok := statModeOf(args)
	if !ok {
		return object.FALSE, nil
	}
	return object.BooleanOf(mode&0o100 != 0), nil
}

func statModeOf(args []object.RubyObject) (uint32, bool) {
	if len(args) != 1 {
		return 0, false
	}
	s, ok := args[0].(*object.String)
	if !ok {
		return 0, false
	}
	info, err := os.Stat(s.Value())
	if err != nil {
		return 0, false
	}
	return uint32(info.Mode().Perm()), true
}

func fileFileQ(_ *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, errorf("evaluator: File.file?: expected 1 arg, got %d", len(args))
	}
	path, ok := args[0].(*object.String)
	if !ok {
		return nil, errorf("evaluator: File.file?: expected String, got %T", args[0])
	}
	info, err := os.Stat(path.Value())
	if err != nil {
		return object.FALSE, nil
	}
	return object.BooleanOf(info.Mode().IsRegular()), nil
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
