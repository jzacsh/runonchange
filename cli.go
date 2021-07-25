package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type parseStage int

const (
	psNumArgs parseStage = iota
	psHelp
	psInvalidFlag
	psCommand
	psWatchTarget
	psFilePattern
	psBadDuration
)

var (
	errHelpRequested       = errors.New("local help docs requested")
	errEmptyArgumentFound  = errors.New("found empty-strng argument")
	errMissingCommand      = errors.New("missing COMMAND")
	errMissingTargets      = errors.New("No DIR_TO_WATCH set")
	errTargetsEmptyStrings = errors.New("No non-empty DIR_TO_WATCH set")
	errMissingShellEnv     = errors.New("$SHELL env variable required")
)

// golang error representing a cli parsing issue.
type parseError struct {
	Stage    parseStage
	errState error

	// Optional error as propogated up to us from a dependency
	Err error
}

func (e parseError) Error() string {
	if e.errState == nil {
		e.errState = fmt.Errorf("parser %s: %w", e.Stage.String(), e.Err)
	}
	return e.errState.Error()
}

func (e parseError) Unwrap() error {
	return e.errState
}

func (e parseError) Is(target error) bool {
	return e.errState == target

	// we'll let golang do an Is() against our child error by calling our Unwrap()
	// internally
}

func (stage *parseStage) String() string {

	switch *stage {
	case psNumArgs:
		return "arg count"
	case psHelp:
		return "help"
	case psInvalidFlag:
		return "invalid flag"
	case psCommand:
		return "COMMAND"
	case psWatchTarget:
		return "DIR_TO_WATCH"
	case psFilePattern:
		return "FILE_PATTERN"
	case psBadDuration:
		return "WAIT_DURATION"
	}
	panic(fmt.Sprintf("unexpected parseStage found, '%d'", int(*stage)))
}

func expectedNonZero(stage parseStage) *parseError {
	return &parseError{
		Stage: stage,
		Err:   fmt.Errorf("expected only non-zero %s values", stage.String()),
	}
}

func parseFilePattern(pattern string) (*regexp.Regexp, *parseError) {
	match, e := regexp.Compile(pattern)
	if e != nil {
		return nil, &parseError{
			Stage: psFilePattern,
			Err:   fmt.Errorf("pattern, '%s': %w", pattern, e),
		}
	}
	return match, nil
}

func validateDirective(d *runDirective) *parseError {
	if len(d.Command) < 1 {
		return &parseError{Stage: psCommand, Err: errMissingCommand}
	}

	if d.Features[flgQuiet] && d.Features[flgDebugOutput] {
		fmt.Fprintf(
			os.Stderr,
			"[debug] you asked for debug mode *and* quiet mode; that's weird, but I'm down... here we go\n")
	}

	if len(d.WatchTargets) < 1 {
		return &parseError{Stage: psWatchTarget, Err: errMissingTargets}
	}

	for _, t := range d.WatchTargets {
		if len(t) > 0 {
			return nil
		}
	}
	return &parseError{
		Stage: psWatchTarget,
		Err:   errTargetsEmptyStrings,
	}
}

func buildBaseDirective() (*runDirective, *parseError) {
	directive := runDirective{
		Features:     make(map[featureFlag]bool),
		WatchTargets: make([]string, len(os.Args)),
		Kills:        make(chan os.Signal, 1),
		Patterns:     make([]matcher, len(os.Args)-2 /*at least drop: exec name, COMMAND*/),
		WaitFor:      defaultWaitTime,
	}
	directive.Features[flgAutoIgnore] = true // TODO encode as "default" somewhere
	directive.WatchTargets[0] = "./"

	shell := os.Getenv("SHELL")
	if len(shell) < 1 {
		return nil, &parseError{
			Stage: psCommand,
			Err:   errMissingShellEnv,
		}
	}
	if _, e := os.Stat(shell); e != nil {
		// we expect shell to be a path name, per:
		//   http://pubs.opengroup.org/onlinepubs/9699919799/basedefs/V1_chap08.html#tag_08
		return nil, &parseError{
			Stage: psCommand,
			Err:   fmt.Errorf("$SHELL: %w", e),
		}
	}
	directive.Shell = shell
	return &directive, nil
}

func parseCli() (*runDirective, error) {
	args := os.Args[1:]
	if len(args) < 1 {
		return nil, parseError{
			Stage: psNumArgs,
			Err:   errMissingCommand,
		}
	}

	directive, e := buildBaseDirective()
	if e != nil {
		return nil, e
	}

	trgtCount := 0
	ptrnCount := 0
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-d":
			directive.Features[flgDebugOutput] = true

		case "-c":
			directive.Features[flgClobberCommands] = true

		case "-R":
			directive.Features[flgRecursiveWatch] = true

		case "-q":
			directive.Features[flgQuiet] = true

		case "-h", "h", "--help", "help":
			return nil, parseError{Stage: psHelp, errState: errHelpRequested}

		case "-w":
			i++
			if len(args) == i {
				return nil, parseError{
					Stage: psBadDuration,
					Err:   fmt.Errorf("no pattern provided to arg #%d, '%s'", i, arg),
				}
			}

			waitFor, e := strconv.Atoi(args[i])
			if e != nil {
				return nil, parseError{
					Stage: psBadDuration,
					Err:   fmt.Errorf("parsing -w duration: %w", e),
				}
			}
			directive.WaitFor = time.Duration(waitFor) * time.Second

		case "-i":
			fallthrough
		case "-r":
			var m matcher
			if arg == "-i" {
				// TODO(zacsh) double-check this works; seems like buggy `case` usage...
				m.IsIgnore = true
			}

			i++
			if len(args) == i {
				return nil, parseError{
					Stage: psFilePattern,
					Err:   fmt.Errorf("no pattern provided to arg #%d, '%s'", i, arg),
				}
			}

			ptrnCount++
			ptrnStr := args[i] // TODO(zacsh) remove this variable
			ptrn, e := parseFilePattern(ptrnStr)
			if e != nil {
				return nil, e
			}

			m.Expr = ptrn
			directive.Patterns[ptrnCount-1] = m

			// positional args: COMMAND, [DIR_TO_WATCH, ...]
		default:
			if len(arg) == 0 {
				return nil, parseError{
					Stage: psInvalidFlag,
					Err:   errEmptyArgumentFound,
				}
			}
			if arg[0] == '-' {
				return nil, parseError{
					Stage: psInvalidFlag,
					Err:   fmt.Errorf("got flag %s", arg),
				}
			}

			if len(directive.Command) == 0 { // arg: COMMAND
				directive.Command = strings.TrimSpace(args[i])
				if len(directive.Command) < 1 {
					return nil, expectedNonZero(psCommand)
				}
				continue // done with COMMAND
			}
			// arg: [DIR_TO_WATCH, ...]

			watchTargetPath := strings.TrimSpace(arg)
			if len(watchTargetPath) < 1 {
				return nil, expectedNonZero(psWatchTarget)
			}
			watchTarget, e := os.Stat(watchTargetPath)
			if e != nil {
				return nil, parseError{Stage: psWatchTarget, Err: e}
			}
			if !watchTarget.IsDir() {
				return nil, parseError{
					Stage: psWatchTarget,
					Err:   fmt.Errorf("target must be a directory, but got: %s", watchTargetPath),
				}
			}
			trgtCount++
			directive.WatchTargets[trgtCount-1] = watchTargetPath
		}
	}

	if ptrnCount == 0 {
		directive.Patterns = nil
	} else {
		directive.Patterns = directive.Patterns[:ptrnCount] // slice off excess
	}

	if trgtCount == 0 {
		directive.WatchTargets = directive.WatchTargets[0:1] // default target
	} else {
		directive.WatchTargets = directive.WatchTargets[:trgtCount] // slice off excess
	}

	if e := validateDirective(directive); e != nil {
		return nil, e
	}

	return directive, nil
}
