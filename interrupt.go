package main

import (
	"fmt"
	"os"
)

func (run *runDirective) handleInterrupt(sig os.Signal) {
	fmt.Fprintf(os.Stderr, "\nCaught %v (%d); Cleaning up `COMMAND`s... ", sig, sig)
	var ex int
	msg := "Done"
	fnd, e := run.cleanupExistant(false /*wait*/)
	if e != nil {
		ex = 1
		msg = fmt.Sprintf("Failed: %v", e)
	} else if !fnd {
		msg = fmt.Sprintf("None found")
	}

	fmt.Fprintf(os.Stderr, "%s\n", msg)

	os.Exit(ex)
}
