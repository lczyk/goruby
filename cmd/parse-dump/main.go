// parse-dump parses a ruby source file (or stdin) and prints the
// resulting AST tree on stdout as YAML (default) or JSON. Each struct
// node carries a `_type` field naming its concrete type so polymorphic
// children are unambiguous.
//
//	parse-dump [--version=X.Y] [--format=yaml|json] [file]
//
// Without a positional argument, reads source from stdin.
//
// Examples:
//
//	parse-dump --version=2.7 fixture.rb
//	echo 'puts 1 + 2' | parse-dump --format=json
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/lczyk/goruby/internal/dumpfmt"
	"github.com/lczyk/goruby/parser"
	"github.com/lczyk/goruby/token"
)

func main() {
	version := flag.String("version", "", "target ruby version (e.g. 2.7); empty = latest")
	format := flag.String("format", "yaml", "output format: yaml | json")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: parse-dump [--version=X.Y] [--format=yaml|json] [file]")
		flag.PrintDefaults()
	}
	flag.Parse()

	fmtKind, err := dumpfmt.ParseFormat(*format)
	if err != nil {
		die("format:", err)
	}

	var verOpt []parser.Option
	if *version != "" {
		v, err := token.ParseVersion(*version)
		if err != nil {
			die("parse version:", err)
		}
		verOpt = append(verOpt, parser.WithVersion(v))
	}

	filename, src := readInput()

	prog, err := parser.ParseFile(filename, src, parser.ParseComments, verOpt...)
	if err != nil {
		die("parse:", err)
	}

	if err := dumpfmt.Encode(os.Stdout, fmtKind, dumpfmt.TypedTree(prog)); err != nil {
		die("encode:", err)
	}
}

func readInput() (filename string, src []byte) {
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
	filename = flag.Arg(0)
	data, err := os.ReadFile(filename)
	if err != nil {
		die("read input:", err)
	}
	return filename, data
}

func die(prefix string, err error) {
	fmt.Fprintln(os.Stderr, "parse-dump:", prefix, err)
	os.Exit(1)
}
