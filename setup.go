package main

import (
	"fmt"
	"github.com/fsnotify/fsnotify"
)

// Entry point for application
//
// TODO: outline what we do at a high level
func (run *runDirective) setup() error {
	watcher, e := fsnotify.NewWatcher()
	if e != nil {
		return fmt.Errorf("starting FS watchers: %v", e)
	}
	run.fsWatcher = watcher

	fsEvents := make(chan fsnotify.Event)
	go run.watchFSEvents(fsEvents)
	go run.handleFSEvents(fsEvents)

	dirCount, e := run.registerDirectoriesToWatch()
	if e != nil {
		return fmt.Errorf("registering FS watchers: %v", e)
	}
	run.reportEstablishedWatches(dirCount)

	// Start an initial run before we even get FS events.
	go run.maybeRun(nil /*event*/, true /*msgStdout*/)

	return nil
}
