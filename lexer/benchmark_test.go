package lexer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lczyk/goruby/token"
)

func BenchmarkLexRealFiles(b *testing.B) {
	var files []string
	dirs := []string{
		"../internal/integrationtest/testdata/gems",
		"../internal/integrationtest/testdata/ruby",
	}
	for _, dir := range dirs {
		filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".rb") {
				return err
			}
			files = append(files, path)
			return nil
		})
	}

	b.ResetTimer()
	b.ReportAllocs()

	var totalBytes int64
	var totalTokens int64

	for i := 0; i < b.N; i++ {
		for _, f := range files {
			src, err := os.ReadFile(f)
			if err != nil {
				b.Fatal(err)
			}
			totalBytes += int64(len(src))
			l := New(string(src))
			for l.HasNext() {
				tok := l.NextToken()
				_ = tok
				totalTokens++
				if tok.Type == token.EOF {
					break
				}
			}
		}
	}

	b.SetBytes(totalBytes / int64(b.N))
	b.ReportMetric(float64(totalTokens)/float64(b.N), "tokens/op")
}

func BenchmarkLexEscapes(b *testing.B) {
	// Long string packed with multi-char escape sequences -- exercises consumeEscape.
	const src = "\"\\u{3042}\\x41\\C-a\\M-\\n\\u{1F600 1F601}\\x7e\\c?\\M-\\C-\\xFF\\u{10FFFF}\""
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		l := New(src)
		for l.HasNext() {
			tok := l.NextToken()
			if tok.Type == token.EOF {
				break
			}
		}
	}
}

func BenchmarkLexInterpolation(b *testing.B) {
	// String with many #{} interpolations -- exercises interp stack push/pop.
	// 50 nested interpolations.
	const src = "\"a#{" +
		"b#{" +
		"c#{" +
		"d#{" +
		"e#{" +
		"f#{" +
		"g#{" +
		"h#{" +
		"i#{" +
		"j#{" +
		"k#{" +
		"l#{" +
		"m#{" +
		"n#{" +
		"o#{" +
		"p#{" +
		"q#{" +
		"r#{" +
		"s#{" +
		"t#{" +
		"u#{" +
		"v#{" +
		"w#{" +
		"x#{" +
		"y#{" +
		"z#{" +
		"42" +
		"}" +
		"}" +
		"}" +
		"}" +
		"}" +
		"}" +
		"}" +
		"}" +
		"}" +
		"}" +
		"}" +
		"}" +
		"}" +
		"}" +
		"}" +
		"}" +
		"}" +
		"}" +
		"}" +
		"}" +
		"}" +
		"}" +
		"}" +
		"}" +
		"}\""
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		l := New(src)
		for l.HasNext() {
			tok := l.NextToken()
			if tok.Type == token.EOF {
				break
			}
		}
	}
}

func BenchmarkLexHeredocBody(b *testing.B) {
	// Interpolating heredoc with escapes and interpolations -- exercises
	// the full heredoc content path (delimiter matching, interpolation, escapes).
	const src = "<<EOS\n" +
		"hello #{name} world\n" +
		"line \\u{41} here\n" +
		"some \\x7e text\n" +
		"and #{more} stuff\n" +
		"EOS\n"
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		l := New(src)
		for l.HasNext() {
			tok := l.NextToken()
			if tok.Type == token.EOF {
				break
			}
		}
	}
}

func BenchmarkLexIdentifierStream(b *testing.B) {
	// Stream of identifiers -- exercises lexIdentifier + LookupIdent hot path.
	const src = "foo bar baz qux quux corge grault garply waldo fred plugh xyzzy thud " +
		"alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi " +
		"omicron pi rho sigma tau upsilon phi chi psi omega one two three four five " +
		"hello world example sample test demo prototype benchmark profile trace " +
		"ruby goruby lexer parser token identifier keyword lookup loop state function"
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		l := New(src)
		for l.HasNext() {
			tok := l.NextToken()
			if tok.Type == token.EOF {
				break
			}
		}
	}
}

func BenchmarkLexKeywords(b *testing.B) {
	// Stream of Ruby keywords -- exercises token.LookupIdent keyword path.
	const src = "def class module if else elsif unless while until for in do end " +
		"begin rescue ensure case when then return next break yield super self nil " +
		"true false and or not defined alias undef BEGIN END __FILE__ __LINE__"
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		l := New(src)
		for l.HasNext() {
			tok := l.NextToken()
			if tok.Type == token.EOF {
				break
			}
		}
	}
}

func BenchmarkLexSingleQuoted(b *testing.B) {
	// Single-quoted string -- exercises lexSingleQuoteString hot path.
	const src = "'hello world this is a simple single-quoted string'"
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		l := New(src)
		for l.HasNext() {
			tok := l.NextToken()
			if tok.Type == token.EOF {
				break
			}
		}
	}
}

func BenchmarkLexRegexBody(b *testing.B) {
	// Regex with escapes and interpolation -- exercises lexRegexContent path.
	const src = "/foo \\u{41} bar \\x7e #{x} baz \\C-a qux \\M-\\n/ix"
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		l := New(src)
		for l.HasNext() {
			tok := l.NextToken()
			if tok.Type == token.EOF {
				break
			}
		}
	}
}

func BenchmarkLexPercentLiteral(b *testing.B) {
	// Percent literal with paired delimiters -- exercises lexPercentContent
	// depth tracking and escape consumption.
	const src = "%Q{hello \\u{41} world #{name} \\x7e}"
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		l := New(src)
		for l.HasNext() {
			tok := l.NextToken()
			if tok.Type == token.EOF {
				break
			}
		}
	}
}

func BenchmarkLexSquigHeredoc(b *testing.B) {
	// Squiggy heredoc -- exercises stripSquigInterpBody indent stripping
	// and the squig heredoc content path.
	const src = "<<~EOS\n" +
		"    hello #{name}\n" +
		"      world\n" +
		"  EOS\n"
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		l := New(src)
		for l.HasNext() {
			tok := l.NextToken()
			if tok.Type == token.EOF {
				break
			}
		}
	}
}

func BenchmarkLexFloatLiteral(b *testing.B) {
	// Float literals with fraction, exponent, and suffixes --
	// exercises lexFloatFraction + lexFloatExponent paths.
	const src = "1.5e+10r 2.5e-3i .5r 1e10 3.14"
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		l := New(src)
		for l.HasNext() {
			tok := l.NextToken()
			if tok.Type == token.EOF {
				break
			}
		}
	}
}

func BenchmarkLexComment(b *testing.B) {
	// Hash comment -- exercises commentLexer path.
	const src = "# this is a comment with some text that goes on for a while\n"
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		l := New(src)
		for l.HasNext() {
			tok := l.NextToken()
			if tok.Type == token.EOF {
				break
			}
		}
	}
}
