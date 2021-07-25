package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"syscall" // TODO(zacsh) important to use x/syscall/unix explicitly?
	"time"

	"github.com/fatih/color"
	"github.com/fsnotify/fsnotify"
)

var magicFileRegexp *regexp.Regexp = regexp.MustCompile(`^(\.\w.*sw[a-z]|4913)$`)

type matcher struct {
	Expr     *regexp.Regexp
	IsIgnore bool
}

func (m matcher) String() string {
	status := "RESTR"
	if m.IsIgnore {
		status = "IGNOR"
	}
	return fmt.Sprintf("[%s]'%v'", status, m.Expr)
}

func (run *runDirective) maybeRun(
	event *fsnotify.Event, stdOut bool) (bool, error) {
	run.RunMux.Lock()
	defer run.RunMux.Unlock()

	if run.isRecent() {
		return false, nil
	}

	run.Birth = make(chan os.Process, 1)
	run.Death = make(chan error, 1)
	run.LastRun = time.Now()
	run.LastFin = time.Time{}

	if stdOut {
		msg := fmt.Sprintf("startup")
		if event != nil {
			msg = fmt.Sprintf("%s on %s", event.Op, event.Name)
		}
		fmt.Printf("\n%s %s ...\n",
			color.YellowString("handling"),
			msg)
	}

	if run.Features[flgClobberCommands] {
		// Try to actually clobber first, if needed (`_` signal)
		if _, e := run.cleanupExtant(true /*wait*/); e != nil {
			return false, fmt.Errorf("trying clobber of last run: %v", e)
		}
	}
	run.runAsync(stdOut)
	return true, nil
}

func (run *runDirective) runAsync(stdOut bool) bool {
	go run.execAsync(stdOut)
	defer close(run.Birth)
	select {
	case p := <-run.Birth:
		run.Living = &p
	}
	return true
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
		run.Living = nil
		run.LastFin = time.Now()
		if msgStdout {
			run.messageDeath(e)
		}
		run.Death <- e
	}()

	// TODO(zacsh) this worker is going to perform some needless thrashing;
	// redesign things to get rid of this; in fact the majority of data in
	// beahvior.go feel unnecessary (Birth, Death, Living, etc.)
	go func() {
		for {
			if run.Cmd.Process != nil {
				run.Birth <- *run.Cmd.Process
				break
			}
		}
	}()
}

func (run *runDirective) isRecent() bool {
	since := run.WaitFor
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

	// Attempt to append a reason to the end of our summary
	var maybeErr string
	if e != nil {
		maybeErr = fmt.Sprintf(
			"\t:  %s",
			color.New(color.Bold, color.FgRed).Sprintf(e.Error()))
	}

	// Summarize death
	fmt.Printf("%s%s in %v.%s\n",
		maybeLn,
		color.YellowString("done"),
		run.LastFin.Sub(run.LastRun),
		maybeErr)
}

// Watches for - and emits to `out` - any applicable filesystem events.
func (run *runDirective) watchFSEvents(out chan fsnotify.Event) {

	for {
		select {
		case e := <-run.fsWatcher.Events:
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

			out <- e
		case err := <-run.fsWatcher.Errors:
			die(exFsevent, err)
		}
	}
}

// Given applicable filesystem events on `in`, runs COMMAND (per --help) for
// each if appropriate, and exits runonchange is shutting down.
func (run *runDirective) handleFSEvents(in chan fsnotify.Event) {
	for {
		select {
		case sig := <-run.Kills:
			run.gracefulCleanup(sig) // Shutdown all of runonchange
		case <-run.Death:
			if !run.Features[flgClobberCommands] {
				continue
			}
			fmt.Fprintf(os.Stderr,
				"\t%s: command died on its own\n",
				color.New(color.Bold, color.FgBlue).Sprintf("warning"))

		case ev := <-in:
			if run.Living != nil && !run.Features[flgClobberCommands] {
				run.tick(tickDropStillRunning)
				continue
			}

			ran, err := run.maybeRun(&ev, true /*msgStdout*/)
			if !ran {
				run.tick(tickClobberUnnecessary)
			}
			if err != nil {
				run.tick(tickClobberFailed)
			}
		}
	}
}
