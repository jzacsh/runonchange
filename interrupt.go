package main

import (
	"fmt"
	"os"
	"syscall" // TODO(zacsh) important to use x/syscall/unix explicitly?
)

// Exit runonchange as gracefully as possible, cleaning up as we go.
func (run *runDirective) gracefulCleanup(sig os.Signal) {
	fmt.Fprintf(os.Stderr,
		"\nCaught %v (%d); starting graceful shutdown...\n", sig, sig)

	var exitStatus int
	var explainAttempt = func(e error, wasNoop bool) string {
		if e != nil {
			if exitStatus == 0 {
				exitStatus = 1
			}
			return fmt.Sprintf("Failed: %v", e)
		} else if wasNoop {
			return fmt.Sprintf("Wasn't neeeded")
		} else {
			return "Done"
		}
	}

	fmt.Fprintf(os.Stderr, " [graceful shutdown]: cleaning up `COMMAND`s...")
	found, e := run.cleanupExistant(false /*wait*/)
	fmt.Fprintf(os.Stderr, "%s\n", explainAttempt(e, !found /*wasNoop*/))

	fmt.Fprintf(os.Stderr, " [graceful shutdown]: cleaning up filesystem watchers...")
	e = run.fsWatcher.Close()
	fmt.Fprintf(os.Stderr, "%s\n", explainAttempt(e, false /*wasNoop*/))

	os.Exit(exitStatus)
}

// Tries to kill any existant COMMANDs still running
// returns indication of whether attempt was made and its errors:
//   true if any existed (ie: any cleanup was necessary)
//   error if cleanup failed
func (run *runDirective) cleanupExistant(wait bool) (existed bool, fail error) {
	existed = run.Living != nil
	if !existed {
		return
	}

	fmt.Fprintf(os.Stderr, " PGID=%d... ", run.Living.Pid)
	if fail = syscall.Kill(-run.Living.Pid, syscall.SIGKILL); fail != nil {
		fmt.Fprintf(os.Stderr,
			"failed to kill exec's pgroup[%d]: %s\n",
			run.Living.Pid, fail)
		return
	}

	if wait {
		// TODO(zacsh) utilize os.Process.Wait() method
		// eg:   s, e := run.Living.Wait()
		<-run.Death
	}
	return
}
