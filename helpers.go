package termtest

import (
	"os"
	"os/exec"
)

type cmdExit struct {
	ProcessState *os.ProcessState
	Err          error
}

// waitForCmdExit turns process.Wait() into a channel so that it can be used within a select{} statement
func waitForCmdExit(cmd *exec.Cmd) chan cmdExit {
	exit := make(chan cmdExit, 1)
	go func() {
		ps, err := cmd.Process.Wait()
		exit <- cmdExit{ProcessState: ps, Err: err}
	}()
	return exit
}

// getIndex returns the given index from the given slice, or the fallback if the index does not exist
func getIndex[T any](v []T, i int, fallback T) T {
	if i > len(v)-1 {
		return fallback
	}
	return v[i]
}

// isClosed checks if the given channel is closed
func isClosed[T any](c chan T) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}
