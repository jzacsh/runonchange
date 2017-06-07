package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type parseStage int

const (
	psNumArgs parseStage = iota
	psHelp
	psCommand
	psWatchTarget
	psFilePattern
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
	case psHelp:
		return "help"
	case psCommand:
		return "COMMAND"
	case psWatchTarget:
		return "DIR_TO_WATCH"
	case psFilePattern:
		return "FILE_PATTERN"
	}
	panic(fmt.Sprintf("unexpected parseStage found, '%d'", int(stage)))
}

func expectedNonZero(stage parseStage) *parseError {
	return &parseError{
		Stage:   stage,
		Message: fmt.Sprintf("expected non-zero %s as argument", parseStageStr(stage)),
	}
}

func parseFilePattern(pattern string) (*regexp.Regexp, *parseError) {
	match, e := regexp.Compile(pattern)
	if e != nil {
		return nil, &parseError{
			Stage:   psFilePattern,
			Message: fmt.Sprintf("pattern, '%s': %s", pattern, e),
		}
	}
	return match, nil
}

func parseCli() (*runDirective, *parseError) {
	args := os.Args[1:]
	if len(args) < 1 {
		return nil, &parseError{
			Stage:   psNumArgs,
			Message: "at least COMMAND argument needed",
		}
	}

	cmd := strings.TrimSpace(args[0])
	if len(cmd) < 1 {
		return nil, expectedNonZero(psCommand)
	}

	if cmd == "-h" || cmd == "h" || cmd == "--help" || cmd == "help" {
		return nil, &parseError{Stage: psHelp}
	}

	directive := runDirective{
		Command:  cmd,
		Features: make(map[featureFlag]bool),
	}
	directive.Features[flgAutoIgnore] = true // TODO encode as "default" somewhere

	shell := os.Getenv("SHELL")
	if len(shell) < 1 {
		return nil, &parseError{
			Stage:   psCommand,
			Message: "$SHELL env variable required",
		}
	}

	if _, e := os.Stat(shell); e != nil {
		// we expect shell to be a path name, per:
		//   http://pubs.opengroup.org/onlinepubs/9699919799/basedefs/V1_chap08.html#tag_08
		return nil, &parseError{
			Stage:   psCommand,
			Message: fmt.Sprintf("$SHELL: %s", e),
		}
	}
	directive.Shell = shell

	defer func(d *runDirective) {
		if len(d.WatchTargets) > 0 {
			return
		}

		d.WatchTargets = []string{"./"}
	}(&directive)

	if len(args) == 1 {
		return &directive, nil
	}

	optionals := args[1:]
	directive.Patterns = make([]matcher, len(optionals))
	directive.WatchTargets = make([]string, len(optionals))
	trgtCount := 0
	ptrnCount := 0
	for i := 0; i < len(optionals); i++ {
		arg := optionals[i]
		switch arg {
		case "-i":
			fallthrough
		case "-r":
			var m matcher
			if arg == "-i" {
				m.IsIgnore = true
			}

			i++
			ptrnCount++
			ptrnStr := optionals[i]
			ptrn, e := parseFilePattern(ptrnStr)
			if e != nil {
				return nil, e
			}

			m.Expr = ptrn
			directive.Patterns[ptrnCount-1] = m
		default:
			watchTargetPath := strings.TrimSpace(arg)
			if len(watchTargetPath) < 1 {
				return nil, expectedNonZero(psWatchTarget)
			}
			watchTarget, e := os.Stat(watchTargetPath)
			if e != nil {
				return nil, &parseError{Stage: psWatchTarget, Message: e.Error()}
			}
			if !watchTarget.IsDir() {
				return nil, &parseError{
					Stage:   psWatchTarget,
					Message: fmt.Sprintf("must be a directory"),
				}
			}
			watchPath, e := filepath.Abs(watchTargetPath)
			if e != nil {
				return nil, &parseError{
					Stage:   psWatchTarget,
					Message: fmt.Sprintf("expanding path: %s", e),
				}
			}
			trgtCount++
			directive.WatchTargets[trgtCount-1] = watchPath
		}
	}

	if ptrnCount == 0 {
		directive.Patterns = nil
	} else {
		directive.Patterns = directive.Patterns[:ptrnCount] // slice off excess
	}

	if trgtCount != 0 {
		directive.WatchTargets = directive.WatchTargets[:trgtCount] // slice off excess
	}

	return &directive, nil
}
