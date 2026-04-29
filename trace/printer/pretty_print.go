package printer

import (
	"fmt"
	"strings"

	"github.com/lczyk/goruby/trace"
)

// for j := len(message.Stack) - 1; j >= 0; j-- {
// 	// for j := len(message.Stack) - 1; j >= 0; j-- {
// 	fmt.Printf("%s:", message.Stack[j])
// }
// fmt.Printf(" %s\n", message.Message)

type Flavour int

const (
	ONELINE Flavour = iota
	MULTILINE
)

func PrettySPrint(message trace.Message, flavour ...Flavour) string {
	if len(flavour) == 0 {
		flavour = append(flavour, ONELINE)
	}

	var out string
	switch flavour[0] {
	case ONELINE:
		out = prettyPrintOneline(message)
	case MULTILINE:
		out = prettyPrintMultiline(message)
	default:
		panic("Unknown flavour")
	}
	return out
}

func PrettyPrint(message trace.Message, flavour ...Flavour) {
	fmt.Print(PrettySPrint(message, flavour...))
}

// flavours

func prettyPrintOneline(message trace.Message) string {
	return message.String()
}

func prettyPrintMultiline(message trace.Message) string {
	var out strings.Builder
	stack := message.Stack()
	for j := 0; j < len(stack); j++ {
		out.WriteString(strings.Repeat(" ", max(j-1, 0))) // indent
		if j > 0 {
			out.WriteString("\u2514") // down-right angle
		}
		out.WriteString(string(stack[j]))
		if j < len(stack)-1 {
			out.WriteString("\n")
		}
	}
	out.WriteString(fmt.Sprintf(": %s\n", message.Message))
	return out.String()
}
