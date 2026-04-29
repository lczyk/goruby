package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"runtime/pprof"
	"strings"

	flags "github.com/jessevdk/go-flags"
	vinfo "github.com/lczyk/goruby/internal/version"
	"github.com/lczyk/goruby/interpreter"
	ver "github.com/lczyk/version/go"
	pkgerrors "github.com/pkg/errors"
)

type Options struct {
	Cpuprofile string   `long:"cpuprofile" description:"write cpu profile to file"`
	Eval       []string `short:"e" description:"one line of script. Several -e's allowed. Omit [programfile]"`
	TraceParse bool     `long:"trace-parse" description:"trace parsing"`
	TraceEval  bool     `long:"trace-eval" description:"trace evaluation"`
	Args       struct {
		ProgramFile string   `positional-arg-name:"programfile"`
		Rest        []string `positional-arg-name:"args"`
	} `positional-args:"yes"`
}

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			fmt.Println(ver.FormatVersion(vinfo.Version, vinfo.CommitSHA, vinfo.BuildDate, vinfo.BuildInfo))
			os.Exit(0)
		}
	}

	var opts Options
	parser := flags.NewParser(&opts, flags.Default)
	parser.Name = "goruby"
	parser.Usage = "[OPTIONS] [programfile] [args...]"
	if _, err := parser.Parse(); err != nil {
		var flagsErr *flags.Error
		if errors.As(err, &flagsErr) && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
		os.Exit(1)
	}

	if opts.Cpuprofile != "" {
		f, err := os.Create(opts.Cpuprofile)
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

	if opts.Args.ProgramFile == "" && len(opts.Eval) == 0 {
		log.Println("No program files specified")
		os.Exit(1)
	}

	interp := interpreter.NewInterpreterEx(opts.Args.Rest)
	if opts.TraceParse {
		interp.SetTraceParse(true)
	}
	if opts.TraceEval {
		interp.SetTraceEval(true)
	}

	if len(opts.Eval) != 0 {
		input := strings.Join(opts.Eval, "\n")
		if _, err := interp.Interpret("", input); err != nil {
			fmt.Printf("%v\n", pkgerrors.Cause(err))
			os.Exit(1)
		}
		return
	}

	fileBytes, err := os.ReadFile(opts.Args.ProgramFile)
	if err != nil {
		log.Printf("Error while opening program file: %T:%v\n", err, err)
		os.Exit(1)
	}
	if _, err := interp.Interpret(opts.Args.ProgramFile, fileBytes); err != nil {
		printError(err)
		os.Exit(1)
	}
}

func printError(err error) {
	fmt.Printf("%T : %v\n", pkgerrors.Cause(err), err)
}
