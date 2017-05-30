package main

import (
	"fmt"
	"os"
	"regexp"
)

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

func (e parseError) Error() string {
	return fmt.Sprintf("parse: %s: %s", parseStageStr(e.Stage), e.Message)
}

func parseStageStr(stage parseStage) string {
	switch stage {
	case psNumArgs:
		return "arg count"
	case psCommand:
		return "COMMAND"
	case psWatchTarget:
		return "DIR_TO_WATCH"
	case psInvertMatch:
		return "FILE_IGNORE_PATTERN"
	}
	panic(fmt.Sprintf("unexpected parseStage found, '%d'", int(stage)))
}

func expectedNonZero(stage parseStage) parseError {
	return parseError{
		Stage:   stage,
		Message: fmt.Sprintf("expected non-zero %s as argument", parseStageStr(stage)),
	}
}

func parseCli() (runDirective, error) {
	args := os.Args[1:]
	if len(args) < 1 {
		return runDirective{}, &parseError{
			Stage:   psNumArgs,
			Message: "at least COMMAND argument needed",
		}
	}

	buildCmd := args[0]
	if len(buildCmd) < 1 {
		return runDirective{}, expectedNonZero(psCommand)
	}

	directive := runDirective{BuildCmd: buildCmd}
	if len(args) > 1 {
		watchTargetPath := args[1]
		if len(watchTargetPath) < 1 {
			return runDirective{}, expectedNonZero(psWatchTarget)
		}
		watchTarget, e := os.Stat(watchTargetPath)
		if e != nil {
			return runDirective{}, parseError{Stage: psWatchTarget, Message: e.Error()}
		}
		if !watchTarget.IsDir() {
			return runDirective{}, parseError{
				Stage:   psWatchTarget,
				Message: fmt.Sprintf("%s must be a directory", parseStageStr(psWatchTarget)),
			}
		}
		directive.WatchTarget = watchTarget

		if len(args) > 2 {
			invertMatch, e := regexp.Compile(args[2])
			if e != nil {
				return runDirective{}, &parseError{
					Stage: psInvertMatch,
					Message: fmt.Sprintf(
						"reading %s pattern: %s", parseStageStr(psInvertMatch), e),
				}
			}
			directive.InvertMatch = invertMatch
		}
	}

	return directive, nil
}
