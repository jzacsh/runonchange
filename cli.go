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
	return fmt.Sprintf("parse: %v: %s", e.Stage, e.Message)
}

func (stage *parseStage) String() string {
	switch *stage {
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
	panic(fmt.Sprintf("unexpected parseStage found, '%d'", int(*stage)))
}

func usage() string {
	return fmt.Sprintf(
		`Runs COMMAND everytime filesystem events happen under DIR_TO_WATCH.

  Usage:  COMMAND [-c] [-i|-r FILE_PATTERN] [DIR_TO_WATCH, ...]

  Description:
    This program watches filesystem events under DIR_TO_WATCH. When an event
    occurs, there is an associated file that caused the event. Those are the
    files, whose basename(1), FILE_PATTERNs are compared against. Except as
    described by -r and -i, said file system events under DIR_TO_WATCH trigger
    COMMAND to be run in the current $SHELL.

  Arguments:
    DIR_TO_WATCH: indicates the directory whose ancestor file events should
    trigger COMMAND to be run. Defaults to the current working directory.
    Multiple directories can be passed.

    DIR_TO_WATCH arguments must be the last on the commandline.

  Flags:
    -c: indicates long-running COMMANDs should be killed when newer triggering
    events are received.

    -i FILE_PATTERN: only run COMMAND if match is not made (invert/ignore)
    -r FILE_PATTERN: only run COMMAND if match is made

      FILE_PATTERN is a regular expression used to match against files whose
      events are as described by the two flags: -i (ignore), -r (restrict).

      If -i than events whose files match FILE_PATTERN will be ignored, thus not
      triggering COMMAND when COMMAND would normally run.

      If -r than only events whose files match FILE_PATTERN can trigger COMMAND.

      Valid FILE_PATTERN strings are those accepted by:
        https://golang.org/pkg/regexp/#Compile
`)
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

	fmt.Fprintf(os.Stderr, "%s error: %s\n", reasonStr, e)
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
		Command:      cmd,
		Features:     make(map[featureFlag]bool),
		WatchTargets: []string{"./"},
		Kills:        make(chan os.Signal, 1),
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

	if len(args) == 1 {
		return &directive, nil
	}

	optionals := args[1:]
	directive.Patterns = make([]matcher, len(optionals))
	trgtCount := 0
	ptrnCount := 0
	for i := 0; i < len(optionals); i++ {
		arg := optionals[i]
		switch arg {
		case "-c":
			directive.Features[flgClobberCommands] = true
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
			if trgtCount == 0 {
				// overwrite default if we've been given any explicitly
				directive.WatchTargets = make([]string, len(optionals))
			}

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
