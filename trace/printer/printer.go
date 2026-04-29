package printer

import (
	"fmt"
	"strings"

	"github.com/lczyk/goruby/trace"
)

func NewTracePrinter() func(trace.Node) error {
	indent := 0
	return func(n trace.Node) error {
		switch n := n.(type) {
		case *trace.Enter:
			if n.Name == trace.START_NODE {
				return nil
			}
			fmt.Printf("%s > %s\n", strings.Repeat(".", indent*2), n.Name)
			indent++
		case *trace.Exit:
			if n.Name == trace.END_NODE {
				return nil
			}
			indent--
			fmt.Printf("%s < %s\n", strings.Repeat(".", indent*2), n.Name)
		case *trace.Message:
			fmt.Printf("%s @ %s\n", strings.Repeat(".", indent*2), n.Message)
		default:
			panic(fmt.Sprintf("unknown node type: %T", n))
		}
		return nil
	}
}
