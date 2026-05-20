// lex-dump tokenises a ruby source file (or stdin) and prints one
// token at a time on stdout. YAML output (default) is a single top-level
// sequence; JSON output is JSONL (one compact object per line).
//
//	lex-dump [--version=X.Y] [--format=yaml|json] [file]
//
// Without a positional argument, reads source from stdin.
//
// Examples:
//
//	lex-dump --version=2.7 fixture.rb
//	echo 'a + b' | lex-dump --format=json
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/lczyk/goruby/internal/dumpfmt"
	"github.com/lczyk/goruby/lexer"
	"github.com/lczyk/goruby/token"
)

// tokenEntry is the per-token shape emitted to YAML / JSON. Decoupled
// from token.Token so the output schema stays under our control even
// if the internal struct evolves.
type tokenEntry struct {
	Pos     int    `json:"pos"`
	End     int    `json:"end"`
	Type    string `json:"type"`
	Literal string `json:"literal"`
}

func main() {
	version := flag.String("version", "", "target ruby version (e.g. 2.7); empty = latest")
	format := flag.String("format", "yaml", "output format: yaml | json (json is jsonl)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: lex-dump [--version=X.Y] [--format=yaml|json] [file]")
		flag.PrintDefaults()
	}
	flag.Parse()

	fmtKind, err := dumpfmt.ParseFormat(*format)
	if err != nil {
		die("format:", err)
	}

	var opts []lexer.Option
	if *version != "" {
		v, err := token.ParseVersion(*version)
		if err != nil {
			die("parse version:", err)
		}
		opts = append(opts, lexer.WithVersion(v))
	}

	src := readInput()
	l := lexer.NewBytes(src, opts...)
	srcStr := string(src)

	var items []any
	for {
		tok := l.NextToken()
		items = append(items, tokenEntry{
			Pos:     tok.Pos,
			End:     tok.EndPos(),
			Type:    tok.Type.String(),
			Literal: tok.LitOf(srcStr),
		})
		if tok.Type == token.EOF {
			break
		}
	}

	if err := dumpfmt.EncodeStream(os.Stdout, fmtKind, items); err != nil {
		die("encode:", err)
	}
}

func readInput() []byte {
	if flag.NArg() == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			die("read stdin:", err)
		}
		return data
	}
	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}
	data, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		die("read input:", err)
	}
	return data
}

func die(prefix string, err error) {
	fmt.Fprintln(os.Stderr, "lex-dump:", prefix, err)
	os.Exit(1)
}
