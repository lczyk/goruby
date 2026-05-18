package parser

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// BenchmarkParseRealFiles parses every .rb under the integration test corpus.
// Companion to BenchmarkLexRealFiles in lexer/ -- gives an end-to-end baseline
// for parser throughput against realistic code shapes.
func BenchmarkParseRealFiles(b *testing.B) {
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
	if len(files) == 0 {
		b.Skip("no .rb files in corpus")
	}

	srcs := make(map[string][]byte, len(files))
	var totalBytes int64
	for _, f := range files {
		buf, err := os.ReadFile(f)
		if err != nil {
			b.Fatal(err)
		}
		srcs[f] = buf
		totalBytes += int64(len(buf))
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, f := range files {
			_, _ = ParseFile(f, srcs[f], 0)
		}
	}
	b.SetBytes(totalBytes)
}

// runBench is the boilerplate harness for the synthetic-source benchmarks.
func runBench(b *testing.B, src string) {
	buf := []byte(src)
	b.SetBytes(int64(len(buf)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseFile("bench.rb", buf, 0)
	}
}

// BenchmarkParseDeepParens exercises pratt parsing recursion depth.
// Fuzz hangs cluster around inputs that pile up nested parens / unary ops --
// each level is one parseExpression frame plus a precedence comparison.
func BenchmarkParseDeepParens(b *testing.B) {
	const depth = 200
	runBench(b, strings.Repeat("(", depth)+"1"+strings.Repeat(")", depth))
}

// BenchmarkParseDeepUnary stacks prefix operators -- separate hotspot from
// parens b/c it goes through parsePrefixExpression rather than parseParen.
func BenchmarkParseDeepUnary(b *testing.B) {
	runBench(b, strings.Repeat("!", 500)+"x")
}

// BenchmarkParseBinopChain hits the precedence-climb path with a long flat
// associative chain (no nesting). Stresses peek/advance + infix dispatch.
func BenchmarkParseBinopChain(b *testing.B) {
	parts := make([]string, 500)
	for i := range parts {
		parts[i] = "1"
	}
	runBench(b, strings.Join(parts, " + "))
}

// BenchmarkParseAssignmentChain pounds the assignment path that surfaced the
// nil-Right bug -- right-associative recursion through parseAssignment.
func BenchmarkParseAssignmentChain(b *testing.B) {
	parts := make([]string, 200)
	for i := range parts {
		parts[i] = "a"
	}
	runBench(b, strings.Join(parts, " = ")+" = 1")
}

// BenchmarkParseCallChain measures left-recursive ContextCallExpression
// rewriting -- `a.b.c....z`. Each dot rebuilds the call node.
func BenchmarkParseCallChain(b *testing.B) {
	parts := make([]string, 400)
	for i := range parts {
		parts[i] = "a"
	}
	runBench(b, strings.Join(parts, "."))
}

// BenchmarkParseRescueBlock exercises parseRescueBlock / parseExceptionHandlingBlock
// -- the path the recent crash came through.
func BenchmarkParseRescueBlock(b *testing.B) {
	var sb strings.Builder
	sb.WriteString("begin\n  x\n")
	for i := 0; i < 50; i++ {
		sb.WriteString("rescue E")
		sb.WriteString(strings.Repeat("A", 3))
		sb.WriteString(" => e\n  y\n")
	}
	sb.WriteString("end\n")
	runBench(b, sb.String())
}

// BenchmarkParseHashLiteral stresses parseHashLiteral + symbol/key parsing.
func BenchmarkParseHashLiteral(b *testing.B) {
	parts := make([]string, 200)
	for i := range parts {
		parts[i] = "k: 1"
	}
	runBench(b, "{"+strings.Join(parts, ", ")+"}")
}

// BenchmarkParseArrayLiteral -- array elements feed parseExpression repeatedly.
func BenchmarkParseArrayLiteral(b *testing.B) {
	parts := make([]string, 500)
	for i := range parts {
		parts[i] = "1"
	}
	runBench(b, "["+strings.Join(parts, ", ")+"]")
}

// BenchmarkParseManyDefs exercises top-level def parsing repeatedly.
func BenchmarkParseManyDefs(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		sb.WriteString("def foo\n  1 + 2\nend\n")
	}
	runBench(b, sb.String())
}

// BenchmarkParseStringInterpolation -- repeated #{...} surfaces lexer/parser
// handoff cost inside dstrings.
func BenchmarkParseStringInterpolation(b *testing.B) {
	var sb strings.Builder
	sb.WriteString(`"`)
	for i := 0; i < 100; i++ {
		sb.WriteString("a#{1+2}")
	}
	sb.WriteString(`"`)
	runBench(b, sb.String())
}

// BenchmarkParseFuzzCorpus replays the captured fuzz corpus -- mirrors what
// the fuzzer actually spends time on (slow-but-bounded inputs included).
func BenchmarkParseFuzzCorpus(b *testing.B) {
	entries, err := os.ReadDir("testdata/fuzz/FuzzParse")
	if err != nil {
		b.Skip("no fuzz corpus")
	}
	var srcs [][]byte
	var totalBytes int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		buf, err := os.ReadFile(filepath.Join("testdata/fuzz/FuzzParse", e.Name()))
		if err != nil {
			continue
		}
		// Fuzz corpus files start with "go test fuzz v1\nstring(...)\n";
		// crude extraction -- skip the header, strip the wrapper.
		s := string(buf)
		idx := strings.Index(s, "string(\"")
		if idx < 0 {
			continue
		}
		end := strings.LastIndex(s, "\")")
		if end <= idx {
			continue
		}
		raw, err := unquoteGoString(s[idx+len("string("):end+1])
		if err != nil {
			continue
		}
		srcs = append(srcs, []byte(raw))
		totalBytes += int64(len(raw))
	}
	if len(srcs) == 0 {
		b.Skip("no usable corpus entries")
	}
	b.SetBytes(totalBytes)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, src := range srcs {
			_, _ = ParseFile("fuzz.rb", src, 0)
		}
	}
}

func unquoteGoString(s string) (string, error) {
	return strconv.Unquote(s)
}
