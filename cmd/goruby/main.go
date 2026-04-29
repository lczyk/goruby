package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/pprof"
	"strings"

	"github.com/lczyk/goruby/interpreter"
	"github.com/pkg/errors"
)

type multiString []string

func (m multiString) String() string {
	out := ""
	for _, s := range m {
		out += s
	}
	return out
}
func (m *multiString) Set(s string) error {
	*m = append(*m, s)
	return nil
}

var onelineScripts multiString
var trace_parse bool
var trace_eval bool
var cpuprofile string = ""

func printError(err error) {
	// fmt.Printf("%v\n", errors.Cause(err))
	fmt.Printf("%T : %v\n", errors.Cause(err), err)
}

func main() {
	flag.StringVar(&cpuprofile, "cpuprofile", "", "write cpu profile to file")
	flag.Var(&onelineScripts, "e", "one line of script. Several -e's allowed. Omit [programfile]")
	flag.BoolVar(&trace_parse, "trace-parse", false, "trace parsing")
	flag.BoolVar(&trace_eval, "trace-eval", false, "trace parsing")
	version := flag.Bool("version", false, "print version")
	flag.Parse()
	if *version {
		fmt.Println("goruby version 0.1.0")
		os.Exit(0)
	}
	if cpuprofile != "" {
		f, err := os.Create(cpuprofile)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		defer func() {
			if err := f.Close(); err != nil {
				log.Fatal("could not close CPU profile: ", err)
			}
		}()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}
	args := flag.Args()
	if len(args) == 0 && len(onelineScripts) == 0 {
		log.Println("No program files specified")
		os.Exit(1)
	}
	interpreter := interpreter.NewInterpreterEx(args[1:])
	if trace_parse {
		interpreter.SetTraceParse(true)
	}
	if trace_eval {
		interpreter.SetTraceEval(true)
	}
	if len(onelineScripts) != 0 {
		input := strings.Join(onelineScripts, "\n")
		_, err := interpreter.Interpret("", input)
		if err != nil {
			fmt.Printf("%v\n", errors.Cause(err))
			os.Exit(1)
		}
		return
	}
	if len(args) == 0 {
		log.Println("No program files specified")
		os.Exit(1)
	}
	fileBytes, err := os.ReadFile(args[0])
	if err != nil {
		log.Printf("Error while opening program file: %T:%v\n", err, err)
		os.Exit(1)
	}
	_, err = interpreter.Interpret(args[0], fileBytes)
	if err != nil {
		printError(err)
		os.Exit(1)
	}
}
