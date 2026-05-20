// parse-dump parses a ruby source file (or stdin) and prints the
// resulting AST tree on stdout as YAML (default) or JSON. Each struct
// node carries a `_type` field naming its concrete type so polymorphic
// children are unambiguous.
//
//	parse-dump [--ruby-version=X.Y] [--format=yaml|json] [file]
//
// Without a positional argument, reads source from stdin.
//
// Examples:
//
//	parse-dump --version=2.7 fixture.rb
//	echo 'puts 1 + 2' | parse-dump --format=json
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	flags "github.com/jessevdk/go-flags"
	"github.com/lczyk/goruby/internal/dumpfmt"
	"github.com/lczyk/goruby/internal/version"
	"github.com/lczyk/goruby/parser"
	"github.com/lczyk/goruby/token"
	ver "github.com/lczyk/version/go"
)

type Options struct {
	RubyVersion string `long:"ruby-version" description:"target ruby version (e.g. 2.7); empty = latest" value-name:"X.Y"`
	Format      string `long:"format" description:"output format: yaml | json" default:"yaml" value-name:"FMT"`
	Version     bool   `short:"v" long:"version" description:"print version and exit"`
}

func main() {
	var opts Options
	p := flags.NewParser(&opts, flags.Default)
	p.Usage = "[--ruby-version=X.Y] [--format=yaml|json] [file]"
	args, err := p.Parse()
	if err != nil {
		var fe *flags.Error
		if errors.As(err, &fe) && fe.Type == flags.ErrHelp {
			os.Exit(0)
		}
		os.Exit(2)
	}

	if opts.Version {
		fmt.Println(ver.FormatVersion(version.Version, version.CommitSHA, version.BuildDate, version.BuildInfo))
		return
	}

	fmtKind, err := dumpfmt.ParseFormat(opts.Format)
	if err != nil {
		die("format:", err)
	}

	var verOpt []parser.Option
	if opts.RubyVersion != "" {
		v, err := token.ParseVersion(opts.RubyVersion)
		if err != nil {
			die("parse version:", err)
		}
		verOpt = append(verOpt, parser.WithVersion(v))
	}

	filename, src := readInput(args)

	prog, err := parser.ParseFile(filename, src, parser.ParseComments, verOpt...)
	if err != nil {
		die("parse:", err)
	}

	if err := dumpfmt.Encode(os.Stdout, fmtKind, dumpfmt.TypedTree(prog)); err != nil {
		die("encode:", err)
	}
}

func readInput(args []string) (filename string, src []byte) {
	if len(args) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			die("read stdin:", err)
		}
		return "<stdin>", data
	}
	if len(args) > 1 {
		die("usage:", fmt.Errorf("too many positional arguments"))
	}
	filename = args[0]
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
