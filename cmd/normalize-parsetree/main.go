// normalize-parsetree runs `<ruby-bin> --dump=parsetree <input.rb>` and
// writes the normalised dump on stdout. Normalisation strips cosmetic /
// positional noise and semantics-preserving no-op wrappers, and masks
// __LINE__ evaluations so two reformatted-but-equivalent sources compare
// equal.
//
//	normalize-parsetree [--raw] <ruby-bin> <input.rb>
//
// With `--raw`, output keeps the indented tree shape instead of the
// path-prefixed flat form.
//
// Example:
//
//	diff \
//	  <(normalize-parsetree ruby a.rb) \
//	  <(normalize-parsetree ruby b.rb)
//
// Without positional arguments, reads the parsetree dump from stdin and
// applies source-agnostic normalisation only (no __LINE__ masking).
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	flags "github.com/jessevdk/go-flags"
	"github.com/lczyk/goruby/internal/parsetreenorm"
	"github.com/lczyk/goruby/internal/version"
	ver "github.com/lczyk/version/go"
)

type Options struct {
	Raw     bool `long:"raw" description:"skip the flat-form reparse; keep indented tree shape"`
	Version bool `short:"v" long:"version" description:"print version and exit"`
}

func main() {
	var opts Options
	p := flags.NewParser(&opts, flags.Default)
	p.Usage = "[--raw] [<ruby-bin> <input.rb>]"
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
	switch len(args) {
	case 0:
		runStdin(opts.Raw)
	case 2:
		runDump(args[0], args[1], opts.Raw)
	default:
		fmt.Fprintln(os.Stderr, "usage: normalize-parsetree [--raw] [<ruby-bin> <input.rb>]")
		os.Exit(2)
	}
}

func runStdin(raw bool) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		die("read stdin:", err)
	}
	fn := parsetreenorm.Normalize
	if raw {
		fn = parsetreenorm.NormalizeRaw
	}
	if _, err := io.WriteString(os.Stdout, fn(string(data))); err != nil {
		die("write stdout:", err)
	}
}

func runDump(rubyBin, inputPath string, raw bool) {
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
	fn := parsetreenorm.NormalizeWithSource
	if raw {
		fn = parsetreenorm.NormalizeWithSourceRaw
	}
	out := fn(stdout.String(), string(src))
	if _, err := io.WriteString(os.Stdout, out); err != nil {
		die("write stdout:", err)
	}
}

func die(prefix string, err error) {
	fmt.Fprintln(os.Stderr, "normalize-parsetree:", prefix, err)
	os.Exit(1)
}
