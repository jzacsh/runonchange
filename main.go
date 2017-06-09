package main

import (
	"fmt"
	"github.com/fsnotify/fsnotify"
	"os"
	"os/exec"
	"regexp"
	"sync"
	"time"
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

	LastRun time.Time
	RunMux  sync.Mutex
	Cmd     *exec.Cmd
	Birth   chan os.Process
	Living  *os.Process
	Death   chan error
	LastFin time.Time
}

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

	run.watch(watcher)

	// must be async regardless of clobber-mode, else we won't be able to watch
	// for SIGINT, below
	go run.maybeRun(true /*msgStdout*/)

	run.waitForKill()
}
