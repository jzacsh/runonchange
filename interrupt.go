package main

import (
	"fmt"
	"os"
	"os/signal"
)

func (run *runDirective) waitForKill() {
	kills := make(chan os.Signal, 1)
	signal.Notify(kills, os.Interrupt)
	for sig := range kills {
		fmt.Fprintf(os.Stderr, "\nCaught %v (%d); Cleaning up... ", sig, sig)
		var ex int
		msg := "Done"
		if _, e := run.cleanupExistant(false /*wait*/); e != nil {
			ex = 1
			msg = fmt.Sprintf("Failed: %v", e)
		}
		fmt.Fprintf(os.Stderr, "%s\n", msg)
		os.Exit(ex)
	}
}
