package main

import (
	"fmt"
	"github.com/fatih/color"
	"os"
	"path/filepath"
	"strings"
)

func (run *runDirective) registerDirectoriesToWatch() (int, error) {
	count := 0
	recursiveWalkHandler := func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			return nil
		}

		count++
		return run.fsWatcher.Add(path)
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
			if e := run.fsWatcher.Add(t); e != nil {
				return count, e
			}
		}
	}
	return count, nil
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
