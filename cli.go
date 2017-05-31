package main

import (
	"fmt"
	"os"
	"path/filepath"
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

	cmd := args[0]
	if len(cmd) < 1 {
		return runDirective{}, expectedNonZero(psCommand)
	}

	directive := runDirective{Command: cmd}
	directive.Features = make(map[featureFlag]bool)
	directive.Features[flgAutoIgnore] = true // TODO encode as "default" somewhere

	shell := os.Getenv("SHELL")
	if len(shell) < 1 {
		return runDirective{}, &parseError{
			Stage:   psCommand,
			Message: "$SHELL env variable required",
		}
	}

	if _, e := os.Stat(shell); e != nil {
		// we expect shell to be a path name, per:
		//   http://pubs.opengroup.org/onlinepubs/9699919799/basedefs/V1_chap08.html#tag_08
		return runDirective{}, &parseError{
			Stage:   psCommand,
			Message: fmt.Sprintf("$SHELL: %s", e),
		}
	}
	directive.Shell = shell

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
				Message: fmt.Sprintf("must be a directory"),
			}
		}
		watchPath, e := filepath.Abs(watchTargetPath)
		if e != nil {
			return runDirective{}, parseError{
				Stage:   psWatchTarget,
				Message: fmt.Sprintf("expanding path: %s", e),
			}
		}
		directive.WatchTarget = watchPath

		if len(args) > 2 {
			invertMatch, e := regexp.Compile(args[2])
			if e != nil {
				return runDirective{}, &parseError{
					Stage:   psInvertMatch,
					Message: fmt.Sprintf("pattern: %s", e),
				}
			}
			directive.InvertMatch = invertMatch
		}
	}

	return directive, nil
}
