package main

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/fsnotify/fsnotify"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"
)

const defaultWaitTime time.Duration = 2 * time.Second

type featureFlag int

const (
	flgAutoIgnore featureFlag = 1 + iota
	// Particularly useful for VIM flury of events, see:
	//   https://stackoverflow.com/q/10300835/287374
	flgDebugOutput
)

func (flg featureFlag) String() string {
	switch flg {
	case flgAutoIgnore:
		return "flgAutoIgnore"
	case flgDebugOutput:
		return "flgDebugOutput"
	default:
		panic(fmt.Sprintf("unexpected flag, '%d'", int(flg)))
	}
}

type exitReason int

const (
	exCommandline exitReason = 1 + iota
	exWatcher
	exFsevent
)

type runDirective struct {
	Shell       string
	Command     string
	WatchTarget string
	InvertMatch *regexp.Regexp
	Features    map[featureFlag]bool
	LastRun     time.Time
}

func (run *runDirective) Exec(msgStdout bool) error {
	if msgStdout {
		fmt.Printf("%s\t: `%s`\n",
			color.YellowString("running"),
			color.HiRedString(run.Command))
	}

	// TODO(zacsh) find out a shell-agnostic way to run comands (eg: *bash*
	// specifically takes a "-c" flag)
	cmd := exec.Command(run.Shell, "-c", run.Command)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	run.LastRun = time.Time{}
	runError := cmd.Run()
	run.LastRun = time.Now()

	if msgStdout {
		if runError == nil {
			fmt.Printf("%s\n", color.YellowString("done"))
		} else {
			fmt.Printf("%s\t:  %s\n\n",
				color.YellowString("done"),
				color.New(color.Bold, color.FgRed).Sprintf(runError.Error()))
		}
	}
	return runError
}

func (run *runDirective) isOkToRun() bool {
	return !(run.isRunning() || run.hasRunRecently(defaultWaitTime))
}

func (run *runDirective) isRunning() bool { return run.LastRun.IsZero() }

func (run *runDirective) hasRunRecently(since time.Duration) bool {
	return time.Since(run.LastRun) <= since
}

func usage() string {
	return fmt.Sprintf(`Runs a command everytime some filesystem events happen.
  Usage:  COMMAND  [DIR_TO_WATCH  [FILE_IGNORE_PATTERN]]

  DIR_TO_WATCH defaults to the current working directory.
  FILE_IGNORE_PATTERN If provided, is used to match against the basename of the
    exact file whose event has been captured. If FILE_IGNORE_PATTERN expression
    matches said file, COMMAND will not be run.
    Valid arguments are those accepted by https://golang.org/pkg/regexp/#Compile
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

func (c *runDirective) debugStr() string {
	invertMatch := "n/a"
	if c.InvertMatch != nil {
		invertMatch = c.InvertMatch.String()
	}

	var features string
	for k, v := range c.Features {
		if v {
			var sep string
			if len(features) > 0 {
				sep = ", "
			}
			features = fmt.Sprintf("%s%s%s", features, sep, k.String())
		}
	}

	return fmt.Sprintf(`
  run.Command:            "%s"
  run.WatchTarget.Name(): "%s"
  run.InvertMatch:        "%s"
  run.Shell:              "%s"
  run.Features:           %s
  `, c.Command, c.WatchTarget, invertMatch, c.Shell, features)
}

func main() {
	magicFileRegexp := regexp.MustCompile(`^(\.\w.*sw[a-z]|4913)$`)

	run, perr := parseCli()
	if perr != nil {
		if perr.Stage == psHelp {
			fmt.Printf(usage())
			os.Exit(0)
		}

		die(exCommandline, perr)
	}

	watcher, e := fsnotify.NewWatcher()
	if e != nil {
		die(exCommandline, e)
	}
	defer watcher.Close()

	fmt.Printf("Watching `%s`\n", run.WatchTarget)

	if run.Features[flgDebugOutput] {
		fmt.Fprintf(
			os.Stderr,
			"[debug] not yet implemented, but here's what you asked for:\n%s\n",
			run.debugStr())
	}

	run.Exec(true /*msgStdout*/)

	haveActionableEvent := make(chan bool)
	done := make(chan bool)
	go func() {
		for {
			select {
			case e := <-watcher.Events:
				if run.Features[flgDebugOutput] {
					fmt.Fprintf(os.Stderr, "[debug] [%s] %s\n", e.Op.String(), e.Name)
				}

				if run.Features[flgAutoIgnore] {
					if magicFileRegexp.MatchString(filepath.Base(e.Name)) {
						continue
					}
				}

				if run.InvertMatch != nil &&
					run.InvertMatch.MatchString(filepath.Base(e.Name)) {
					continue
				}

				haveActionableEvent <- true
			case err := <-watcher.Errors:
				die(exFsevent, err)
			}
		}
	}()

	go func() {
		for {
			select {
			case <-haveActionableEvent:
				if !run.isOkToRun() {
					fmt.Fprintf(os.Stderr, ".")
					continue
				}

				fmt.Fprintf(os.Stderr, "\n")
				run.Exec(true /*msgStdout*/)
			}
		}
	}()

	if err := watcher.Add(run.WatchTarget); err != nil {
		die(exWatcher, e)
	}
	<-done // hang main
}
