// parse-roundtrip parses a ruby source file (or stdin) and re-emits
// it via ast.Format. By default the re-emitted source goes to stdout,
// which lets `diff` between input and output reveal cosmetic-only
// reformatting. With --check, the command exits non-zero if input and
// output differ and writes a unified diff to stderr.
//
//	parse-roundtrip [--version=X.Y] [--check] [file]
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
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/parser"
	"github.com/lczyk/goruby/token"
)

func main() {
	version := flag.String("version", "", "target ruby version (e.g. 2.7); empty = latest")
	check := flag.Bool("check", false, "exit non-zero on input/output mismatch and write a diff to stderr")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: parse-roundtrip [--version=X.Y] [--check] [file]")
		flag.PrintDefaults()
	}
	flag.Parse()

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

	out := ast.Format(prog)

	if *check {
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
