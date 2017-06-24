package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/fsnotify/fsnotify"
)

const defaultWaitTime time.Duration = 2 * time.Second

var magicFileRegexp *regexp.Regexp = regexp.MustCompile(`^(\.\w.*sw[a-z]|4913)$`)

type featureFlag int

const (
	flgAutoIgnore featureFlag = 1 + iota
	// Particularly useful for VIM flury of events, see:
	//   https://stackoverflow.com/q/10300835/287374
	flgDebugOutput
	flgClobberCommands
	flgRecursiveWatch
)

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
	WaitFor      time.Duration

	LastRun time.Time
	RunMux  sync.Mutex
	Cmd     *exec.Cmd
	Birth   chan os.Process
	Kills   chan os.Signal
	Living  *os.Process
	Death   chan error
	LastFin time.Time
}

func (flg featureFlag) String() string {
	switch flg {
	case flgAutoIgnore:
		return "flgAutoIgnore"
	case flgClobberCommands:
		return "flgClobberCommands"
	case flgRecursiveWatch:
		return "flgRecursiveWatch"
	case flgDebugOutput:
		return "flgDebugOutput"
	default:
		panic(fmt.Sprintf("unexpected flag, '%d'", int(flg)))
	}
}

func (run *runDirective) reportEstablishedWatches(numWatchedDirs int) {
	var recursiveMsg string
	if run.Features[flgRecursiveWatch] {
		var recurseCount string
		if numWatchedDirs > len(run.WatchTargets) {
			recurseCount = fmt.Sprintf("[+%d]", numWatchedDirs-len(run.WatchTargets))
		}
		recursiveMsg = fmt.Sprintf(
			"%s%s ", color.HiRedString("recursively"), recurseCount)
	}

	var clobberMode string
	if run.Features[flgClobberCommands] {
		clobberMode = fmt.Sprintf(" (in %s mode)", color.RedString("clobber"))
	}
	fmt.Printf("%s%s%s:\n\t%s\n",
		recursiveMsg,
		color.HiGreenString("watching"),
		clobberMode,
		strings.Join(run.WatchTargets, ", "))
}

func main() {
	run, perr := parseCli()
	if perr != nil {
		if perr.Stage == psHelp {
			fmt.Printf(usage())
			os.Exit(0)
		}

		die(exCommandline, perr)
	}

	if run.Features[flgDebugOutput] {
		fmt.Fprintf(os.Stderr,
			"[debug] here's what you asked for:\n%s\n",
			run.debugStr())
	}

	watcher, e := fsnotify.NewWatcher()
	if e != nil {
		die(exCommandline, e)
	}
	defer watcher.Close()
	haveActionableEvent := make(chan fsnotify.Event)
	go run.watchFSEvents(watcher, haveActionableEvent)
	go run.handleFSEvents(haveActionableEvent)

	count, e := run.watch(watcher)
	if e != nil {
		die(exWatcher, e)
	}
	run.reportEstablishedWatches(count)

	// must be async regardless of clobber-mode, else we won't be able to watch
	// for SIGINT, below
	go run.maybeRun(nil /*event*/, true /*msgStdout*/)

	signal.Notify(run.Kills, os.Interrupt)

	<-make(chan bool) // hang main
}
