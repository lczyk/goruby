//go:build integration || oracle

package integrationtest

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/goruby/lexer"
	"github.com/lczyk/goruby/parser"
	"github.com/lczyk/goruby/token"
)

const boundaryTSV = "testdata/version-boundaries.tsv"

type boundaryEntry struct {
	minVersion string // "1.9", "2.0", ..., or "any"
	file       string // relative to testdata/
}

func loadBoundaries(t *testing.T) []boundaryEntry {
	t.Helper()
	f, err := os.Open(boundaryTSV)
	assert.NoError(t, err, "open %s", boundaryTSV)
	defer f.Close()

	var entries []boundaryEntry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			t.Fatalf("malformed line in %s: %q", boundaryTSV, line)
		}
		entries = append(entries, boundaryEntry{
			minVersion: parts[0],
			file:       parts[1],
		})
	}
	assert.NoError(t, sc.Err(), "read %s", boundaryTSV)
	return entries
}

// allBoundaryVersions returns every known version boundary for testing.
func allBoundaryVersions() []token.RubyVersion {
	return []token.RubyVersion{
		token.MustParseVersion("1.9"),
		token.MustParseVersion("2.0"),
		token.MustParseVersion("2.1"),
		token.MustParseVersion("2.3"),
		token.MustParseVersion("2.5"),
		token.MustParseVersion("2.6"),
		token.MustParseVersion("2.7"),
		token.MustParseVersion("3.0"),
		token.MustParseVersion("3.1"),
		token.MustParseVersion("3.2"),
		token.MustParseVersion("3.4"),
		token.MustParseVersion("4.0"),
	}
}

func TestVersionBoundariesLex(t *testing.T) {
	entries := loadBoundaries(t)
	versions := allBoundaryVersions()

	for _, entry := range entries {
		src, err := os.ReadFile(filepath.Join("testdata", entry.file))
		assert.NoError(t, err, "read %s", entry.file)

		var minVer token.RubyVersion
		isAny := entry.minVersion == "any"
		if !isAny {
			minVer = token.MustParseVersion(entry.minVersion)
		}

		for _, ver := range versions {
			name := fmt.Sprintf("%s/ruby_%s", filepath.Base(entry.file), ver)
			shouldPass := isAny || ver.AtLeast(minVer)

			t.Run(name, func(t *testing.T) {
				t.Parallel()
				lexErr := runLexWithVersion(string(src), ver)
				if shouldPass && lexErr != nil {
					t.Errorf("expected lex to pass at %s, got: %v", ver, lexErr)
				}
				// We don't assert failure below min version for lexing --
				// some version differences only manifest at parse time.
			})
		}
	}
}

func TestVersionBoundariesParse(t *testing.T) {
	entries := loadBoundaries(t)
	versions := allBoundaryVersions()

	for _, entry := range entries {
		src, err := os.ReadFile(filepath.Join("testdata", entry.file))
		assert.NoError(t, err, "read %s", entry.file)

		var minVer token.RubyVersion
		isAny := entry.minVersion == "any"
		if !isAny {
			minVer = token.MustParseVersion(entry.minVersion)
		}

		for _, ver := range versions {
			name := fmt.Sprintf("%s/ruby_%s", filepath.Base(entry.file), ver)
			shouldPass := isAny || ver.AtLeast(minVer)

			t.Run(name, func(t *testing.T) {
				t.Parallel()
				parseErr := runParseWithVersion(entry.file, string(src), ver)
				if shouldPass && parseErr != nil {
					t.Errorf("expected parse to pass at %s, got: %v", ver, parseErr)
				}
				// TODO: once the parser is version-aware, also assert that
				// parsing FAILS below min version:
				// if !shouldPass && parseErr == nil {
				//     t.Errorf("expected parse to fail at %s, but it passed", ver)
				// }
			})
		}
	}
}

func runLexWithVersion(src string, ver token.RubyVersion) error {
	l := lexer.New(src, lexer.WithVersion(ver))
	for l.HasNext() {
		tok := l.NextToken()
		if tok.Type == token.ILLEGAL {
			return fmt.Errorf("ILLEGAL token at pos %d: %q", tok.Pos, tok.Literal)
		}
		if tok.Type == token.EOF {
			break
		}
	}
	return nil
}

func runParseWithVersion(name, src string, ver token.RubyVersion) error {
	_, err := parser.ParseFile(name, []byte(src), parser.AllErrors, parser.WithVersion(ver))
	return err
}
