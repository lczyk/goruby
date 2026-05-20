// lex-dump tokenises a ruby source file (or stdin) and prints one
// token at a time on stdout. YAML output (default) is a single top-level
// sequence; JSON output is JSONL (one compact object per line).
//
//	lex-dump [--ruby-version=X.Y] [--format=yaml|json] [file]
//
// Without a positional argument, reads source from stdin.
//
// Examples:
//
//	lex-dump --ruby-version=2.7 fixture.rb
//	echo 'a + b' | lex-dump --format=json
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	flags "github.com/jessevdk/go-flags"
	"github.com/lczyk/goruby/internal/dumpfmt"
	"github.com/lczyk/goruby/internal/version"
	"github.com/lczyk/goruby/lexer"
	"github.com/lczyk/goruby/token"
	ver "github.com/lczyk/version/go"
)

type Options struct {
	RubyVersion string `long:"ruby-version" description:"target ruby version (e.g. 2.7); empty = latest" value-name:"X.Y"`
	Format      string `long:"format" description:"output format: yaml | json (json is jsonl)" default:"yaml" value-name:"FMT"`
	Version     bool   `short:"v" long:"version" description:"print version and exit"`
}

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
	var opts0 Options
	p := flags.NewParser(&opts0, flags.Default)
	p.Usage = "[--ruby-version=X.Y] [--format=yaml|json] [file]"
	args, err := p.Parse()
	if err != nil {
		var fe *flags.Error
		if errors.As(err, &fe) && fe.Type == flags.ErrHelp {
			os.Exit(0)
		}
		os.Exit(2)
	}

	if opts0.Version {
		fmt.Println(ver.FormatVersion(version.Version, version.CommitSHA, version.BuildDate, version.BuildInfo))
		return
	}

	fmtKind, err := dumpfmt.ParseFormat(opts0.Format)
	if err != nil {
		die("format:", err)
	}

	var opts []lexer.Option
	if opts0.RubyVersion != "" {
		v, err := token.ParseVersion(opts0.RubyVersion)
		if err != nil {
			die("parse version:", err)
		}
		opts = append(opts, lexer.WithVersion(v))
	}

	src := readInput(args)
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

func readInput(args []string) []byte {
	if len(args) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			die("read stdin:", err)
		}
		return data
	}
	if len(args) > 1 {
		die("usage:", fmt.Errorf("too many positional arguments"))
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		die("read input:", err)
	}
	return data
}

func die(prefix string, err error) {
	fmt.Fprintln(os.Stderr, "lex-dump:", prefix, err)
	os.Exit(1)
}
