package main

import (
	"fmt"
	"github.com/fsnotify/fsnotify"
)

// Entry point for application to start runonchange logic, once preferences and
// settings have been taken care of (parsing CLI flags, basic validation, OS signal
// mgmt) upstream by main.
//
// runonchange logic we need to setup:
// - worker to watch and filter Filesystem events
// - worker to handle filtered events and invoke COMMAND
// - configuration of filesystem event library
// - kick off an initial, sample COMMAND invocation
func (run *runDirective) setup() error {
	watcher, e := fsnotify.NewWatcher()
	if e != nil {
		return fmt.Errorf("starting FS watchers: %v", e)
	}
	run.fsWatcher = watcher

	fsEvents := make(chan fsnotify.Event)
	go func() {
		run.watchFSEvents(fsEvents)
	}()
	go func() {
		run.handleFSEvents(fsEvents)
	}()

	dirCount, e := run.registerDirectoriesToWatch()
	if e != nil {
		return fmt.Errorf("registering FS watchers: %v", e)
	}
	run.reportEstablishedWatches(dirCount)

	// Start an initial run before we even get FS events.
	go run.maybeRun(nil /*event*/, true /*msgStdout*/)

	return nil
}
