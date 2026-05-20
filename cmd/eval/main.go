// eval runs a ruby source file (or stdin) through the goruby
// tree-walking evaluator, writing program output to stdout. Useful
// for one-off debugging of evaluator behaviour without booting MRI.
//
//	eval [--version=X.Y] [file]
//
// Without a positional argument, reads source from stdin. The exit code
// is non-zero if parsing or evaluation fails (error goes to stderr).
//
// Examples:
//
//	eval fixture.rb
//	echo 'puts 1 + 2' | eval
//	eval --version=2.6 fixture.rb
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/lczyk/goruby/evaluator"
	"github.com/lczyk/goruby/object"
	"github.com/lczyk/goruby/parser"
	"github.com/lczyk/goruby/token"
)

func main() {
	version := flag.String("version", "", "target ruby version (e.g. 2.6); empty = latest")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: eval [--version=X.Y] [file]")
		flag.PrintDefaults()
	}
	flag.Parse()

	envOpts := []object.EnvOption{object.WithStdout(os.Stdout)}
	var parseOpts []parser.Option
	if *version != "" {
		v, err := token.ParseVersion(*version)
		if err != nil {
			die("parse version:", err)
		}
		parseOpts = append(parseOpts, parser.WithVersion(v))
		envOpts = append(envOpts, object.WithVersion(v))
	}

	filename, src := readInput()

	prog, err := parser.ParseFile(filename, src, 0, parseOpts...)
	if err != nil {
		die("parse:", err)
	}

	env := object.NewMainEnvironment(envOpts...)
	if _, err := evaluator.Eval(prog, env); err != nil {
		die("eval:", err)
	}
}

func readInput() (string, []byte) {
	if flag.NArg() == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			die("read stdin:", err)
		}
		return "<stdin>", data
	}
	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}
	filename := flag.Arg(0)
	data, err := os.ReadFile(filename)
	if err != nil {
		die("read input:", err)
	}
	return filename, data
}

func die(prefix string, err error) {
	fmt.Fprintln(os.Stderr, "eval:", prefix, err)
	os.Exit(1)
}
