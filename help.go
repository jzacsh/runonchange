package main

import (
	"fmt"
	"time"
)

const version string = "v0.2.1"

const defaultWaitTime time.Duration = 2 * time.Second

func usage() string {
	return fmt.Sprintf(
		`Runs COMMAND everytime filesystem events happen under a DIR_TO_WATCH.

  Usage:  COMMAND [-cdR] [-w WAIT_DURATION] [-i|-r FILE_PATTERN] [DIR_TO_WATCH, ...]

  Description:
	 This program watches filesystem events under DIR_TO_WATCH. When an event
	 occurs, there is an associated file the event originated at. Those are the
	 files whose paths FILE_PATTERNs are compared against.

	 Generally all file system events under DIR_TO_WATCH (with exceptions as
	 documented for -r and -i and -R) will trigger COMMAND. COMMAND will be run
	 in the current $SHELL.

  Arguments:
    DIR_TO_WATCH: indicates the directory whose ancestor file events should
    trigger COMMAND to be run. Defaults to the current working directory.
	  Multiple directories can be passed, so DIR_TO_WATCH arguments must be the
	  last on the commandline.

  General options:
    -d: indicates debugging output should be printed.

    -c: indicates long-running COMMANDs should be killed when newer triggering
    events are received. This is particularly useful if COMMAND is a non-exiting
	  process, like an HTTP server, or perhaps a test suite that takes minutes to
	  run.

    -w WAIT_DURATION: indicates minimum seconds to wait after starting COMMAND,
	  before re-running COMMAND again for new filesystem events. Defaults to %s.

  Filesystem event configuration options:

    -R: indicates a recursive watch should be established under DIR_TO_WATCH.
    That is: COMMAND will be triggered by more than just file events of
    immediate children to DIR_TO_WATCH.

	  File matching options:

    -i FILE_PATTERN: only run COMMAND if match is not made (invert/ignore)
    -r FILE_PATTERN: only run COMMAND if match is made

	  For both -i (ignore) and -r (restrict) the FILE_PATTERN value is a regular
	  expression used to match against a file for which a filesystem events is
	  has caused us to consider running COMMAND. To clarify:

      If -i than events whose files match FILE_PATTERN will be ignored, thus not
      triggering COMMAND when COMMAND would normally run.

      If -r than only events whose files match FILE_PATTERN can trigger COMMAND.

      Valid FILE_PATTERN strings are those accepted by:
        https://golang.org/pkg/regexp/#Compile

  Output while running:

	  Generally the output strives to be self-explanatory and minimal. Minimal so
	  as to stay not compete with the output you likely really care about: the
	  output of COMMAND. Nevertheless some signasl are emitted (eg: when starting
	  a new COMMAND invocation, so  that COMMAND's own output doesn't get
	  confusing).

   Ticks as filesystem event indicators:

	  There is however output that while minimal, is certainly not self
	  explanatory and that's the small tick-marks used to indicate a filesystem
    event has been caught. The tickmarks are documented by their corresponding
    enum values here:
       https://github.com/jzacsh/runonchange/blob/master/tick.go

    The tickmark behavior is worth explaining. It's a desired feature to let the
    user know that runonchange isn't just broken, in case COMMAND is not being
    re-run. Of course that's no guarantee that it's not indeed frozen, but often
    it is sufficient signal to prove nothing is broken, and in fact you're
    receiving the behavior you want.

  Version %s
 		 github.com/jzacsh/runonchange/releases/tag/%s
`, defaultWaitTime, version, version)
}
