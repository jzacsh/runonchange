package main

import (
	"fmt"
	"os"
)

type tickSignal string

const (
	// Received an applicable filesystem event, but previous COMMAND is still
	// running (use -c to clobber previous commands).
	tickDropStillRunning tickSignal = "_"

	// Received an applicable filesystem event, considered handling it by first
	// clobbering previously running COMMAND, but found none still alive or the
	// last event was too recent.
	tickClobberUnnecessary = "-"

	// Received an applicable filesystem event, considered handling it by first
	// clobbering previously running COMMAND, but failed to kill it, so we're
	// unable to handle the event now.
	tickClobberFailed = "e"

	// Received filesystem event but originating file matched -i PATTERN
	tickDropPatternIgnore = "i"

	// Received filesystem event but originating file doesn't match -r PATTERN
	tickDropPatternRestric = "r"
)

func (t tickSignal) String() string {
	return string(t)
}

func (run *runDirective) tick(signal tickSignal) {
	if run.Features[flgQuiet] {
		return
	}
	fmt.Fprintf(os.Stderr, "%s", signal)
}
