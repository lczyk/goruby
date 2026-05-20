// parse-roundtrip parses a ruby source file (or stdin) and re-emits
// it via ast.Format. By default the re-emitted source goes to stdout,
// which lets `diff` between input and output reveal cosmetic-only
// reformatting. With --check, the command exits non-zero if input and
// output differ and writes a unified diff to stderr.
//
//	parse-roundtrip [--ruby-version=X.Y] [--check] [file]
//
// Without a positional argument, reads source from stdin.
//
// Examples:
//
//	parse-roundtrip fixture.rb
//	diff -u fixture.rb <(parse-roundtrip fixture.rb)
//	parse-roundtrip --check fixture.rb
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	flags "github.com/jessevdk/go-flags"
	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/internal/version"
	"github.com/lczyk/goruby/parser"
	"github.com/lczyk/goruby/token"
	ver "github.com/lczyk/version/go"
)

type Options struct {
	RubyVersion string `long:"ruby-version" description:"target ruby version (e.g. 2.7); empty = latest" value-name:"X.Y"`
	Check       bool   `long:"check" description:"exit non-zero on input/output mismatch and write a diff to stderr"`
	Version     bool   `short:"v" long:"version" description:"print version and exit"`
}

func main() {
	var opts Options
	p := flags.NewParser(&opts, flags.Default)
	p.Usage = "[--ruby-version=X.Y] [--check] [file]"
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

	out := ast.Format(prog)

	if opts.Check {
		if out == string(src) {
			return
		}
		writeUnifiedDiff(os.Stderr, filename, string(src), out)
		os.Exit(1)
	}

	if _, err := io.WriteString(os.Stdout, out); err != nil {
		die("write stdout:", err)
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

// writeUnifiedDiff emits a unified diff between a and b, labelling the
// hunks "input" and "output". Minimal implementation -- intended for
// human eyeballing, not patch consumption.
func writeUnifiedDiff(w io.Writer, name, a, b string) {
	bw := bufio.NewWriter(w)
	defer bw.Flush()
	fmt.Fprintf(bw, "--- %s (input)\n", name)
	fmt.Fprintf(bw, "+++ %s (re-emitted)\n", name)
	aLines := strings.Split(a, "\n")
	bLines := strings.Split(b, "\n")
	n := max(len(aLines), len(bLines))
	for i := range n {
		var av, bv string
		if i < len(aLines) {
			av = aLines[i]
		}
		if i < len(bLines) {
			bv = bLines[i]
		}
		if av == bv {
			continue
		}
		if i < len(aLines) {
			fmt.Fprintf(bw, "-%s\n", av)
		}
		if i < len(bLines) {
			fmt.Fprintf(bw, "+%s\n", bv)
		}
	}
}

func die(prefix string, err error) {
	fmt.Fprintln(os.Stderr, "parse-roundtrip:", prefix, err)
	os.Exit(1)
}
