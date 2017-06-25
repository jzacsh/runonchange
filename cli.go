package main

import (
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

type parseError struct {
	Stage   parseStage
	Message string
}

func (e parseError) Error() string {
	return fmt.Sprintf("parser: %s: %s", e.Stage.String(), e.Message)
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

func usage() string {
	return fmt.Sprintf(
		`Runs COMMAND everytime filesystem events happen under a DIR_TO_WATCH.

  Usage:  COMMAND [-cdR] [-w WAIT_DURATION] [-i|-r FILE_PATTERN] [DIR_TO_WATCH, ...]

  Description:
    This program watches filesystem events under DIR_TO_WATCH. When an event
    occurs, there is an associated file that caused the event. Those are the
    files whose paths FILE_PATTERNs are compared against. Except as described by
    -r and -i, said file system events under DIR_TO_WATCH trigger COMMAND to be
    run in the current $SHELL.

  Arguments:
    DIR_TO_WATCH: indicates the directory whose ancestor file events should
    trigger COMMAND to be run. Defaults to the current working directory.
    Multiple directories can be passed.

    DIR_TO_WATCH arguments must be the last on the commandline.

  Flags:
    -d: indicates debugging output should be printed.

    -R: indicates a recursive watch should be established under DIR_TO_WATCH.
    That is: COMMAND will be triggered by more than just file events of
    immediate children to DIR_TO_WATCH.

    -c: indicates long-running COMMANDs should be killed when newer triggering
    events are received.

    -w WAIT_DURATION: indicates minimum seconds to wait after starting COMMAND,
    before re-running COMMAND for new filesystem events. Defaults to %s.

    -i FILE_PATTERN: only run COMMAND if match is not made (invert/ignore)
    -r FILE_PATTERN: only run COMMAND if match is made

      FILE_PATTERN is a regular expression used to match against files whose
      events are as described by the two flags: -i (ignore), -r (restrict).

      If -i than events whose files match FILE_PATTERN will be ignored, thus not
      triggering COMMAND when COMMAND would normally run.

      If -r than only events whose files match FILE_PATTERN can trigger COMMAND.

      Valid FILE_PATTERN strings are those accepted by:
        https://golang.org/pkg/regexp/#Compile
`, defaultWaitTime)
}

func die(reason exitReason, e error) {
	var reasonStr string
	switch reason {
	case exCommandline:
		reasonStr = "usage"
	case exWatcher:
		reasonStr = "watcher"
	case exFsevent:
		reasonStr = "event"
	}

	fmt.Fprintf(os.Stderr, "%s error: %s\n", reasonStr, e.Error())
	os.Exit(int(reason))
}

func expectedNonZero(stage parseStage) *parseError {
	return &parseError{
		Stage:   stage,
		Message: fmt.Sprintf("expected non-zero %v as argument", stage),
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

func validateDirective(d *runDirective) *parseError {
	if len(d.Command) < 1 {
		return &parseError{Stage: psCommand, Message: ""}
	}

	if len(d.WatchTargets) < 1 {
		return &parseError{Stage: psWatchTarget, Message: "No DIR_TO_WATCH set"}
	}

	for _, t := range d.WatchTargets {
		if len(t) > 0 {
			return nil
		}
	}
	return &parseError{
		Stage:   psWatchTarget,
		Message: "No non-empty DIR_TO_WATCH set",
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
	return &directive, nil
}

func parseCli() (*runDirective, *parseError) {
	args := os.Args[1:]
	if len(args) < 1 {
		return nil, &parseError{
			Stage:   psNumArgs,
			Message: "at least COMMAND argument needed",
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

		case "-h", "h", "--help", "help":
			return nil, &parseError{Stage: psHelp}

		case "-w":
			i++
			if len(args) == i {
				return nil, &parseError{
					Stage:   psBadDuration,
					Message: fmt.Sprintf("no pattern provided to arg #%d, '%s'", i, arg),
				}
			}

			waitFor, e := strconv.Atoi(args[i])
			if e != nil {
				return nil, &parseError{Stage: psBadDuration, Message: e.Error()}
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
				return nil, &parseError{
					Stage:   psFilePattern,
					Message: fmt.Sprintf("no pattern provided to arg #%d, '%s'", i, arg),
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
			if arg[0] == '-' {
				return nil, &parseError{
					Stage:   psInvalidFlag,
					Message: fmt.Sprintf("got flag %s", arg),
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
				return nil, &parseError{Stage: psWatchTarget, Message: e.Error()}
			}
			if !watchTarget.IsDir() {
				return nil, &parseError{
					Stage:   psWatchTarget,
					Message: fmt.Sprintf("must be a directory"),
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
