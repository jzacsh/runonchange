package main

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/fsnotify/fsnotify"
	"os"
	"os/exec"
	"path/filepath"
	"syscall" // TODO(zacsh) important to use x/syscall/unix explicitly?
	"time"
)

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
		fmt.Fprintf(os.Stderr,
			"failed to kill exec's pgroup[%d]: %s\n",
			run.Living.Pid, fail)
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

	var maybeErr string
	if e != nil {
		maybeErr = fmt.Sprintf(
			"\t:  %s",
			color.New(color.Bold, color.FgRed).Sprintf(e.Error()))
	}

	fmt.Printf("%s%s in %v.%s\n",
		maybeLn,
		color.YellowString("done"),
		run.LastFin.Sub(run.LastRun),
		maybeErr)
}

func (run *runDirective) watchFSEvents(
	watcher *fsnotify.Watcher,
	out chan fsnotify.Event) {

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

			out <- e
		case err := <-watcher.Errors:
			die(exFsevent, err)
		}
	}
}

func (run *runDirective) handleFSEvents(in chan fsnotify.Event) {
	for {
		select {
		case <-run.Death:
			run.Living = nil
			if !run.Features[flgClobberCommands] {
				continue
			}
			fmt.Fprintf(os.Stderr,
				"\t%s: death was unprovoked\n",
				color.New(color.Bold, color.FgBlue).Sprintf("warning"))

		case <-in:
			if ran, _ := run.maybeRun(true /*msgStdout*/); !ran {
				fmt.Fprintf(os.Stderr, ".")
			}
		}
	}
}
