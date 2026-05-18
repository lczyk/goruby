// normalize-parsetree reads `ruby --dump=parsetree` output on stdin and
// writes the normalised form on stdout. Lets two trees be compared
// structurally, ignoring cosmetic / positional differences and
// semantics-preserving no-op wrappers.
//
//	ruby --dump=parsetree foo.rb | normalize-parsetree
//	diff <(ruby --dump=parsetree a.rb | normalize-parsetree) \
//	     <(ruby --dump=parsetree b.rb | normalize-parsetree)
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/lczyk/goruby/internal/parsetreenorm"
)

func main() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "normalize-parsetree:", err)
		os.Exit(1)
	}
	if _, err := io.WriteString(os.Stdout, parsetreenorm.Normalize(string(data))); err != nil {
		fmt.Fprintln(os.Stderr, "normalize-parsetree:", err)
		os.Exit(1)
	}
}
