// normalize-parsetree runs `<ruby-bin> --dump=parsetree <input.rb>` and
// writes the normalised dump on stdout. Normalisation strips cosmetic /
// positional noise and semantics-preserving no-op wrappers, and masks
// __LINE__ evaluations so two reformatted-but-equivalent sources compare
// equal.
//
//	normalize-parsetree <ruby-bin> <input.rb>
//
// Example:
//
//	diff \
//	  <(normalize-parsetree ruby a.rb) \
//	  <(normalize-parsetree ruby b.rb)
//
// Without arguments, reads the parsetree dump from stdin and applies
// source-agnostic normalisation only (no __LINE__ masking).
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/lczyk/goruby/internal/parsetreenorm"
)

func main() {
	switch len(os.Args) {
	case 1:
		runStdin()
	case 3:
		runDump(os.Args[1], os.Args[2])
	default:
		fmt.Fprintln(os.Stderr, "usage: normalize-parsetree [<ruby-bin> <input.rb>]")
		os.Exit(2)
	}
}

func runStdin() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		die("read stdin:", err)
	}
	if _, err := io.WriteString(os.Stdout, parsetreenorm.Normalize(string(data))); err != nil {
		die("write stdout:", err)
	}
}

func runDump(rubyBin, inputPath string) {
	src, err := os.ReadFile(inputPath)
	if err != nil {
		die("read input:", err)
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(rubyBin, "--disable-gems", "--dump=parsetree", inputPath)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "ruby dump failed:", err)
		fmt.Fprintln(os.Stderr, stderr.String())
		os.Exit(1)
	}
	out := parsetreenorm.NormalizeWithSource(stdout.String(), string(src))
	if _, err := io.WriteString(os.Stdout, out); err != nil {
		die("write stdout:", err)
	}
}

func die(prefix string, err error) {
	fmt.Fprintln(os.Stderr, "normalize-parsetree:", prefix, err)
	os.Exit(1)
}
