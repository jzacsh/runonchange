package main

import (
	"fmt"
	"os"
	"regexp"
)

type exitReason int

const (
	exCommandline exitReason = 1 + iota
)

type runDirective struct {
	BuildCmd    string
	WatchTarget os.FileInfo
	InvertMatch *regexp.Regexp
}

func usage() string {
	return fmt.Sprintf(`Runs a command everytime some filesystem events happen.
  Usage:  COMMAND  [DIR_TO_WATCH  [FILE_IGNORE_PATTERN]]

  DIR_TO_WATCH defaults to the current working directory.
  FILE_IGNORE_PATTERN If provided, is used to match against the exact file whose
    event has been captured. If FILE_IGNORE_PATTERN expression matches said
    file, COMMAND will not be run.
    Valid arguments are those accepted by https://golang.org/pkg/regexp/#Compile
`)
}

func die(reason exitReason, e error) {
	var reasonStr string
	switch reason {
	case exCommandline:
		reasonStr = "usage error"
	}

	fmt.Fprintf(os.Stderr, "%s: %s\n", reasonStr, e)
	os.Exit(int(reason))
}

func (c *runDirective) debugStr() string {
	watchTarg := "n/a"
	if c.WatchTarget != nil {
		watchTarg = c.WatchTarget.Name()
	}

	invertMatch := "n/a"
	if c.InvertMatch != nil {
		invertMatch = c.InvertMatch.String()
	}
	return fmt.Sprintf(`
  cmd.BuildCmd:           "%s"
  cmd.WatchTarget.Name(): "%s"
  cmd.InvertMatch:        "%s"
  `, c.BuildCmd, watchTarg, invertMatch) // TODO(zacsh) remove
}

func main() {
	cmd, e := parseCli()
	if e != nil {
		die(exCommandline, e)
	}

	fmt.Fprintf(
		os.Stderr,
		"[dbg] not yet implemented, but here's what you asked for: %s\n",
		cmd.debugStr())
}
