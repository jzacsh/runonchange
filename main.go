package main

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/fsnotify/fsnotify"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
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

type matcher struct {
	Expr     *regexp.Regexp
	IsIgnore bool
}

type runDirective struct {
	Shell        string
	Command      string
	WatchTargets []string
	Patterns     []matcher
	Features     map[featureFlag]bool

	LastRun time.Time
	RunMux  sync.Mutex
	LastFin time.Time
}

func (m *matcher) String() string {
	status := "RESTR"
	if m.IsIgnore {
		status = "IGNOR"
	}
	return fmt.Sprintf("[%s]: %v", status, *m.Expr)
}

func (run *runDirective) maybeRun(stdOut bool) (bool, error) {
	run.RunMux.Lock()
	defer run.RunMux.Unlock()

	if run.isRecent(defaultWaitTime) {
		return false, nil
	}

	e := run.execCmd(stdOut)
	return true, e
}

func (run *runDirective) execCmd(msgStdout bool) error {
	if msgStdout {
		fmt.Printf("\n%s\t: `%s`\n",
			color.YellowString("running"),
			color.HiRedString(run.Command))
	}

	// TODO(zacsh) find out a shell-agnostic way to run comands (eg: *bash*
	// specifically takes a "-c" flag)
	cmd := exec.Command(run.Shell, "-c", run.Command)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	run.LastRun = time.Now()
	run.LastFin = time.Time{}
	runError := cmd.Run()
	run.LastFin = time.Now()

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

func (run *runDirective) isRecent(since time.Duration) bool {
	return time.Since(run.LastFin) <= since
}

func usage() string {
	return fmt.Sprintf(`Runs a command everytime some filesystem events happen.
  Usage:  COMMAND  [-i|-r FILE_PATTERN] [DIR_TO_WATCH, ...]

  Regular expressions can be used to match against files whose events have been
  as described by the next two flags:

  -i FILE_PATTERN: only run COMMAND if match is not made (invert/ignore)
  -r FILE_PATTERN: only run COMMAND if match is made

    This program watches filesystem events. Thus, when an event occurs, there
    is an associated file that causes that event. FILE_PATTERN tries to match
    that file's basename to FILE_PATTERN. The result of that match is as
    described by the flag preceding FILE_PATTERN, explained above.

    Valid FILE_PATTERN strings are those accepted by:
      https://golang.org/pkg/regexp/#Compile

  DIR_TO_WATCH defaults to just one: the current working directory. Multiple
  directories can be passed.
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
	matchStr := "n/a"
	if len(c.Patterns) > 0 {
		matchStr = fmt.Sprintf("'%v',", c.Patterns[0])
		for _, p := range c.Patterns[1:] {
			matchStr = fmt.Sprintf("%s '%v',", matchStr, p)
		}

		matchStr = fmt.Sprintf(
			"%s", // close off bracket
			matchStr[:len(matchStr)-1 /*chop off trailing comma*/])
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
  run.Command:                "%s"
  run.WatchTargets' Name()s:  [%s]
  run.FilePatterns:           [%s]
  run.Shell:                  "%s"
  run.Features:                %s
  `, c.Command,
		fmt.Sprintf("\n\t%s\n\t", strings.Join(c.WatchTargets, ",\n\t")),
		matchStr,
		c.Shell,
		features)
}

func (run *runDirective) isRejected(chain []matcher, e fsnotify.Event) bool {
	if len(chain) == 0 {
		return false
	}

	for i, p := range chain {
		if p.IsIgnore {
			if p.Expr.MatchString(filepath.Base(e.Name)) {
				if run.Features[flgDebugOutput] {
					fmt.Fprintf(os.Stderr, "IGNR[%d]\n", i)
				} else {
					fmt.Fprintf(os.Stderr, "-")
				}
				return true
			}
		} else {
			if !p.Expr.MatchString(filepath.Base(e.Name)) {
				if run.Features[flgDebugOutput] {
					fmt.Fprintf(os.Stderr, "MISS[%d]\n", i)
				} else {
					fmt.Fprintf(os.Stderr, "_")
				}
				return true
			}
		}
	}
	return false
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

	if run.Features[flgDebugOutput] {
		fmt.Fprintf(os.Stderr, "[debug] here's what you asked for:\n%s\n", run.debugStr())
	}

	run.maybeRun(true /*msgStdout*/)

	haveActionableEvent := make(chan bool)
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

				if run.isRejected(run.Patterns, e) {
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
				if ran, _ := run.maybeRun(true /*msgStdout*/); !ran {
					fmt.Fprintf(os.Stderr, ".")
				}
			}
		}
	}()

	for _, t := range run.WatchTargets {
		if err := watcher.Add(t); err != nil {
			die(exWatcher, e)
		}
	}

	fmt.Printf("%s `%s`\n",
		color.HiGreenString("Watching"),
		strings.Join(run.WatchTargets, ", "))

	<-make(chan bool) // hang main
}
