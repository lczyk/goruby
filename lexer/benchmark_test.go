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
