package main

import (
	"fmt"
	"github.com/fsnotify/fsnotify"
	"os"
	"path/filepath"
	"strings"
)

func (c *runDirective) debugStr() string {
	matchStr := "n/a"
	if len(c.Patterns) > 0 {
		matchStr = fmt.Sprintf("'%v',", c.Patterns[0])
		for _, p := range c.Patterns[1:] {
			matchStr = fmt.Sprintf("%s '%v',", matchStr, p)
		}

		matchStr = fmt.Sprintf(
			"%s", // close off bracket
			matchStr[:len(matchStr)-1 /*chop off trailing comma*/])
	}

	var features string
	for k, v := range c.Features {
		if v {
			var sep string
			if len(features) > 0 {
				sep = ", "
			}
			features = fmt.Sprintf("%s%s%s", features, sep, k.String())
		}
	}

	return fmt.Sprintf(`
  run.Command:                "%s"
  run.WatchTargets' Name()s:  [%s]
  run.FilePatterns:           [%s]
  run.Shell:                  "%s"
  run.Features:                %s
  `, c.Command,
		fmt.Sprintf("\n\t%s\n\t", strings.Join(c.WatchTargets, ",\n\t")),
		matchStr,
		c.Shell,
		features)
}

func (run *runDirective) isRejected(chain []matcher, e fsnotify.Event) bool {
	if len(chain) == 0 {
		return false
	}

	for i, p := range chain {
		if p.IsIgnore {
			if p.Expr.MatchString(filepath.Base(e.Name)) {
				if run.Features[flgDebugOutput] {
					fmt.Fprintf(os.Stderr, "IGNR[%d]\n", i)
				} else {
					fmt.Fprintf(os.Stderr, "-")
				}
				return true
			}
		} else {
			if !p.Expr.MatchString(filepath.Base(e.Name)) {
				if run.Features[flgDebugOutput] {
					fmt.Fprintf(os.Stderr, "MISS[%d]\n", i)
				} else {
					fmt.Fprintf(os.Stderr, "_")
				}
				return true
			}
		}
	}
	return false
}
