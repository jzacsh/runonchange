package main

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/fsnotify/fsnotify"
	"strings"
)

func (run *runDirective) watch(watcher *fsnotify.Watcher) {
	for _, t := range run.WatchTargets {
		if err := watcher.Add(t); err != nil {
			die(exWatcher, err)
		}
	}

	var clobberMode string
	if run.Features[flgClobberCommands] {
		clobberMode = fmt.Sprintf(" (in %s mode)", color.RedString("clobber"))
	}
	fmt.Printf("%s%s `%s`\n",
		color.HiGreenString("Watching"),
		clobberMode,
		strings.Join(run.WatchTargets, ", "))
}
