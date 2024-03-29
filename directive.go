package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/fsnotify/fsnotify"
)

func (c *runDirective) debugStr() string {
	matchStr := "n/a"
	if len(c.Patterns) > 0 {
		matchStr = ""
		for _, p := range c.Patterns {
			matchStr = fmt.Sprintf("%s %v,", matchStr, p)
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
			features = fmt.Sprintf("%s%s%s", features, sep, k)
		}
	}

	return fmt.Sprintf(`
  run.Command:                "%s"
  run.WatchTargets' Name()s:  [%s
  ]
  run.FilePatterns:           [%s]
  run.Shell:                  "%s"
  run.WaitFor:                 %s
  run.Features:                %s
  `, c.Command,
		fmt.Sprintf("\n\t%s", strings.Join(c.WatchTargets, ",\n\t")),
		matchStr,
		c.Shell,
		c.WaitFor,
		features)
}

func (run *runDirective) isRejected(chain []matcher, e fsnotify.Event) bool {
	if len(chain) == 0 {
		return false
	}

	for i, p := range chain {
		if p.IsIgnore {
			if p.Expr.MatchString(e.Name) {
				if run.Features[flgDebugOutput] {
					fmt.Fprintf(os.Stderr, "IGNR[%d]\n", i)
				} else {
					run.tick(tickDropPatternIgnore)
				}
				return true
			}
		} else {
			if !p.Expr.MatchString(e.Name) {
				if run.Features[flgDebugOutput] {
					fmt.Fprintf(os.Stderr, "MISS[%d]\n", i)
				} else {
					run.tick(tickDropPatternRestric)
				}
				return true
			}
		}
	}
	return false
}
