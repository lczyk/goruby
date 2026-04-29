package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/goruby/readline"
	flags "github.com/jessevdk/go-flags"
	vinfo "github.com/lczyk/goruby/internal/version"
	"github.com/lczyk/goruby/repl"
	ver "github.com/lczyk/version/go"
)

type Options struct {
	NoEcho   bool `long:"noecho" description:"suppress echo of evaluated output"`
	NoPrompt bool `long:"noprompt" description:"suppress prompt"`
}

var opts Options

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			fmt.Println(ver.FormatVersion(vinfo.Version, vinfo.CommitSHA, vinfo.BuildDate, vinfo.BuildInfo))
			os.Exit(0)
		}
	}

	parser := flags.NewParser(&opts, flags.Default)
	parser.Name = "girb"
	parser.Usage = "[OPTIONS]"
	if _, err := parser.Parse(); err != nil {
		var flagsErr *flags.Error
		if errors.As(err, &flagsErr) && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
		os.Exit(1)
	}
	os.Exit(startRepl())
}

func startRepl() int {
	rm := rawMode{StdinFd: int(os.Stdin.Fd())}
	config := &readline.Config{
		InterruptPrompt:   "^C",
		EOFPrompt:         "\n",
		HistorySearchFold: true,
		FuncMakeRaw:       rm.enter,
		FuncExitRaw:       rm.exit,
	}
	l, err := readline.NewEx(config)
	if err != nil {
		log.Printf("Error initializing readlines: %v\n", err)
		return 1
	}
	defer l.Close()
	lNoInterrupt := &ignoreInterrupt{l}

	var out io.Writer = lNoInterrupt
	if opts.NoEcho {
		out = io.Discard
	}
	var prompt repl.Prompt = lNoInterrupt
	if opts.NoPrompt {
		prompt = repl.PromptFunc(discardPrompt)
	}

	r := repl.New(lNoInterrupt, out, prompt)
	if err := r.Start(); err != nil {
		log.Printf("Error within repl: %v\n", err)
		return 1
	}
	return 0
}

func discardPrompt(string) {}

type ignoreInterrupt struct {
	*readline.Instance
}

func (i *ignoreInterrupt) Readline() (string, error) {
	line, err := i.Instance.Readline()
	if err == readline.ErrInterrupt {
		return line, nil
	}
	return line, err
}

type rawMode struct {
	StdinFd int
	state   *readline.State
}

func (r *rawMode) enter() (err error) {
	r.state, err = readline.MakeRaw(r.StdinFd)
	return err
}

func (r *rawMode) exit() error {
	if r.state == nil {
		return nil
	}
	return readline.Restore(r.StdinFd, r.state)
}
