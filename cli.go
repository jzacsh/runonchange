package main

import "fmt"

type parseStage int

const (
	psNumArgs parseStage = iota
	psCommand
	psWatchTarget
	psInvertMatch
)

type parseError struct {
	Stage   parseStage
	Message string
}

func (e *parseError) Error() string {
	var stageStr string
	switch e.Stage {
	case psNumArgs:
		stageStr = "arg count"
	case psCommand:
		stageStr = "COMMAND"
	case psWatchTarget:
		stageStr = "DIR_TO_WATCH"
	case psInvertMatch:
		stageStr = "FILE_IGNORE_PATTERN"
	}
	return fmt.Sprintf("parse: %s: %s", stageStr, e.Message)
}

func parseCli() (runDirective, error) {
	return runDirective{}, &parseError{Stage: psCommand, Message: "Not yet implemented"}
}
