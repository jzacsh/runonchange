package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

func (run *runDirective) watch(watcher *fsnotify.Watcher) (int, error) {
	count := 0
	recursiveWalkHandler := func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			return nil
		}

		count++
		return watcher.Add(path)
	}

	for _, t := range run.WatchTargets {
		if run.Features[flgRecursiveWatch] {
			if e := filepath.Walk(t, recursiveWalkHandler); e != nil {
				return count, e
			}
		} else {
			if run.Features[flgDebugOutput] {
				fmt.Fprintf(os.Stderr, "[debug] w: %s\n", t)
			}

			count++
			if e := watcher.Add(t); e != nil {
				return count, e
			}
		}
	}
	return count, nil
}
