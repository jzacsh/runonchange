package main

import (
	"fmt"
	"os"
	"os/signal"
	"regexp"
)

var magicFileRegexp *regexp.Regexp = regexp.MustCompile(`^(\.\w.*sw[a-z]|4913)$`)

type matcher struct {
	Expr     *regexp.Regexp
	IsIgnore bool
}

func main() {
	run, perr := parseCli()
	if perr != nil {
		if perr.Stage == psHelp {
			fmt.Printf(usage())
			os.Exit(0)
		}

		die(exCommandline, perr)
	}

	if run.Features[flgDebugOutput] {
		fmt.Fprintf(os.Stderr,
			"[debug] here's what you asked for:\n%s\n",
			run.debugStr())
	}

	if e := run.setup(); e != nil {
		die(exWatcher, e)
	}

	signal.Notify(run.Kills, os.Interrupt)

	<-make(chan bool) // hang main
}
