package main

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/fsnotify/fsnotify"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall" // TODO(zacsh) important to use x/syscall/unix explicitly?
	"time"
)

const defaultWaitTime time.Duration = 2 * time.Second

type featureFlag int

const (
	flgAutoIgnore featureFlag = 1 + iota
	// Particularly useful for VIM flury of events, see:
	//   https://stackoverflow.com/q/10300835/287374
	flgDebugOutput
	flgClobberCommands
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
	Cmd     *exec.Cmd
	Birth   chan os.Process
	Living  *os.Process
	Death   chan error
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

	run.Birth = make(chan os.Process, 1)
	run.Death = make(chan error, 1)
	run.LastRun = time.Now()
	run.LastFin = time.Time{}
	if run.Features[flgClobberCommands] {
		return run.runAsync(stdOut), nil
	} else {
		return run.runSync(stdOut)
	}
}

func (run *runDirective) runSync(stdOut bool) (bool, error) {
	run.execAsync(stdOut)
	e := <-run.Death // block
	close(run.Birth)
	return true, e
}

// Tries to kill any existant COMMANDs still running
// returns indication of whether attempt was made and its errors:
//   true if any existed (ie: any cleanup was necessary)
//   error if cleanup failed
func (run *runDirective) cleanupExistant(wait bool) (existed bool, fail error) {
	existed = run.Living != nil
	if !existed {
		return
	}

	if fail = syscall.Kill(-run.Living.Pid, syscall.SIGKILL); fail != nil {
		fmt.Fprintf(os.Stderr, "failed to kill exec's pgroup: %s\n", fail)
		return
	}

	if wait {
		// TODO(zacsh) utilize os.Process.Wait() method
		<-run.Death
	}
	return
}

func (run *runDirective) runAsync(stdOut bool) bool {
	if _, e := run.cleanupExistant(true /*wait*/); e != nil {
		return false
	}

	go run.execAsync(stdOut)
	select {
	case p := <-run.Birth:
		run.Living = &p
		close(run.Birth)
		return true
	}
}

func (run *runDirective) execAsync(msgStdout bool) {
	if msgStdout {
		fmt.Printf("\n%s\t: `%s`\n",
			color.YellowString("running"),
			color.HiRedString(run.Command))
	}

	// TODO(zacsh) find out a shell-agnostic way to run comands (eg: *bash*
	// specifically takes a "-c" flag)
	run.Cmd = exec.Command(run.Shell, "-c", run.Command)
	run.Cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	run.Cmd.Stdout = os.Stdout
	run.Cmd.Stderr = os.Stderr

	go func() {
		e := run.Cmd.Run()
		run.LastFin = time.Now()
		if msgStdout {
			run.messageDeath(e)
		}
		run.Death <- e
	}()

	for {
		if run.Cmd != nil && run.Cmd.Process != nil {
			run.Birth <- *run.Cmd.Process
			break
		}
	}
}

func (run *runDirective) isRecent(since time.Duration) bool {
	if run.Features[flgClobberCommands] {
		since *= 2
	}

	return time.Since(run.LastRun) <= since ||
		time.Since(run.LastFin) <= since
}

func (run *runDirective) messageDeath(e error) {
	var maybeLn string
	if run.Features[flgClobberCommands] {
		maybeLn = "\n"
	}

	if e == nil {
		fmt.Printf("%s%s\n", maybeLn, color.YellowString("done"))
		return
	}

	fmt.Printf("%s%s\t:  %s\n\n",
		maybeLn,
		color.YellowString("done"),
		color.New(color.Bold, color.FgRed).Sprintf(e.Error()))
}

func usage() string {
	return fmt.Sprintf(`Runs a command everytime some filesystem events happen.
  Usage:  COMMAND -c  [-i|-r FILE_PATTERN] [DIR_TO_WATCH, ...]

  If -c is passed, then long-running COMMAND will be killed when newer
  triggering events are received.

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
		fmt.Fprintf(os.Stderr,
			"[debug] here's what you asked for:\n%s\n",
			run.debugStr())
	}

	haveActionableEvent := make(chan fsnotify.Event)
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

				haveActionableEvent <- e
			case err := <-watcher.Errors:
				die(exFsevent, err)
			}
		}
	}()

	go func() {
		for {
			select {
			case e := <-run.Death:
				run.Living = nil
				if run.Features[flgClobberCommands] {
					fmt.Fprintf(os.Stderr,
						"%s: captured natural death, unprovoked; output was: %v\n",
						color.HiYellowString("WARNING"),
						e)
				}

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

	run.maybeRun(true /*msgStdout*/)
	fmt.Printf("%s `%s`\n",
		color.HiGreenString("Watching"),
		strings.Join(run.WatchTargets, ", "))

	kills := make(chan os.Signal, 1)
	signal.Notify(kills, os.Interrupt)
	for sig := range kills {
		fmt.Fprintf(os.Stderr, "\nCaught %v (%d); Cleaning up... ", sig, sig)
		ex := 0
		if _, e := run.cleanupExistant(false /*wait*/); e != nil {
			ex = 1
			fmt.Fprintf(os.Stderr, "Failed: %v\n", e)
		} else {
			fmt.Fprintf(os.Stderr, "Done\n")
		}
		os.Exit(ex)
	}
}
