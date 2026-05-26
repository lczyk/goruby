// goruby runs a ruby program through the goruby tree-walking
// evaluator. Mirrors the upstream goruby binary: takes either a
// program file, one or more -e oneliners, or stdin. Positional
// arguments after the program file populate the ARGV constant.
//
//	goruby [-e script]... [-I dir]... [--ruby-version=X.Y] [programfile [argv...]]
//
// Examples:
//
//	goruby hello.rb
//	goruby -e 'puts 1 + 2'
//	goruby -e 'x = 1' -e 'puts x + 2'
//	goruby -Ilib script.rb
//	goruby --ruby-version=2.6 hello.rb
//	goruby interp.rb program.input
//	echo 'puts 1 + 2' | goruby
//
// The RUBYLIB environment variable is honoured: directories listed (separated
// by the platform path separator) are prepended to $LOAD_PATH after any -I
// entries.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	flags "github.com/jessevdk/go-flags"
	"github.com/lczyk/goruby/evaluator"
	"github.com/lczyk/goruby/internal/version"
	"github.com/lczyk/goruby/object"
	"github.com/lczyk/goruby/parser"
	"github.com/lczyk/goruby/token"
	ver "github.com/lczyk/version/go"
)

type Options struct {
	Scripts      []string `short:"e" long:"execute" description:"one line of script (repeatable); omit [programfile]" value-name:"SCRIPT"`
	IncludePaths []string `short:"I" long:"include" description:"prepend DIR to $LOAD_PATH (repeatable)" value-name:"DIR"`
	RubyVersion  string   `long:"ruby-version" description:"target ruby version (e.g. 2.6); empty = latest" value-name:"X.Y"`
	Version      bool     `short:"v" long:"version" description:"print version and exit"`
}

func main() {
	var opts Options
	parser0 := flags.NewParser(&opts, flags.Default|flags.PassAfterNonOption)
	parser0.Usage = "[--ruby-version=X.Y] [-e script]... [-I dir]... [programfile [argv...]]"
	args, err := parser0.Parse()
	if err != nil {
		var flagsErr *flags.Error
		if errors.As(err, &flagsErr) && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
		os.Exit(2)
	}

	if opts.Version {
		fmt.Println(ver.FormatVersion(version.Version, version.CommitSHA, version.BuildDate, version.BuildInfo))
		return
	}

	envOpts := []object.EnvOption{object.WithStdout(os.Stdout)}
	var parseOpts []parser.Option
	if opts.RubyVersion != "" {
		v, err := token.ParseVersion(opts.RubyVersion)
		if err != nil {
			die("parse version:", err)
		}
		parseOpts = append(parseOpts, parser.WithVersion(v))
		envOpts = append(envOpts, object.WithVersion(v))
	}

	filename, src, argv := readInput(opts.Scripts, args)

	prog, err := parser.ParseFile(filename, src, 0, parseOpts...)
	if err != nil {
		die("parse:", err)
	}

	envOpts = append(envOpts, object.WithARGV(argv))
	env := object.NewMainEnvironment(envOpts...)
	// $PROGRAM_NAME (alias $0) -- MRI sets to the running script's path.
	// Some libraries gate their driver via `if __FILE__ == $PROGRAM_NAME`.
	env.SetGlobal("$PROGRAM_NAME", object.NewString(filename))
	env.SetGlobal("$0", object.NewString(filename))
	if paths := collectLoadPaths(opts.IncludePaths); len(paths) > 0 {
		if err := seedLoadPath(env, paths, parseOpts); err != nil {
			die("seed load path:", err)
		}
	}
	if _, err := evaluator.Eval(prog, env); err != nil {
		die("eval:", err)
	}
}

// collectLoadPaths returns the directories to prepend to $LOAD_PATH,
// in the order they should appear. CLI -I entries come first (matches
// MRI: `-I` wins over RUBYLIB), then RUBYLIB entries split on the
// platform path separator.
func collectLoadPaths(cliIncludes []string) []string {
	var out []string
	out = append(out, cliIncludes...)
	if rubylib := os.Getenv("RUBYLIB"); rubylib != "" {
		for _, p := range strings.Split(rubylib, string(os.PathListSeparator)) {
			if p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// seedLoadPath bootstraps $LOAD_PATH and unshifts the given paths in
// reverse order so the first entry ends up at index 0. Runs as a small
// ruby preamble so it goes through the same code path user scripts
// would use.
func seedLoadPath(env *object.Environment, paths []string, parseOpts []parser.Option) error {
	var b strings.Builder
	for i := len(paths) - 1; i >= 0; i-- {
		fmt.Fprintf(&b, "$LOAD_PATH.unshift(%q)\n", paths[i])
	}
	prog, err := parser.ParseFile("(load-path)", []byte(b.String()), 0, parseOpts...)
	if err != nil {
		return err
	}
	_, err = evaluator.Eval(prog, env)
	return err
}

func readInput(scripts, args []string) (string, []byte, []string) {
	if len(scripts) > 0 {
		return "-e", []byte(strings.Join(scripts, "\n")), args
	}
	if len(args) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			die("read stdin:", err)
		}
		return "<stdin>", data, nil
	}
	filename := args[0]
	data, err := os.ReadFile(filename)
	if err != nil {
		die("read input:", err)
	}
	return filename, data, args[1:]
}

func die(prefix string, err error) {
	fmt.Fprintln(os.Stderr, "goruby:", prefix, err)
	os.Exit(1)
}
