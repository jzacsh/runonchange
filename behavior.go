package main

// Behavioral interface for a given invocation of runonchange

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Flag indicating a change to default behaviors.
type featureFlag int

const (
	flgNoDefaultIgnorePattern featureFlag = 1 + iota
	// Particularly useful for VIM flury of events, see:
	//   https://stackoverflow.com/q/10300835/287374
	flgDebugOutput
	flgClobberCommands
	flgRecursiveWatch
	flgQuiet
)

func (flg featureFlag) String() string {
	switch flg {
	case flgNoDefaultIgnorePattern:
		return "flgNoDefaultIgnorePattern"
	case flgClobberCommands:
		return "flgClobberCommands"
	case flgRecursiveWatch:
		return "flgRecursiveWatch"
	case flgQuiet:
		return "flgQuiet"
	case flgDebugOutput:
		return "flgDebugOutput"
	default:
		panic(fmt.Sprintf("unexpected flag, '%d'", int(flg)))
	}
}

// Encapsulates a given invocation - both its configuration and its currently
// live interanl state.
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

	fsWatcher *fsnotify.Watcher
}
