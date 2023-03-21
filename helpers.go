package termtest

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type voidWriter struct{}

func (v voidWriter) Write(p []byte) (n int, err error) { return len(p), nil }

var neverGonnaHappen = time.Hour * 24 * 365 * 100

var lineSep = "\n"

func init() {
	if runtime.GOOS == "windows" {
		lineSep = "\r\n"
	}
}

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

func unwrapErrorMessage(err error) string {
	msg := []string{}
	for err != nil {
		msg = append(msg, err.Error())
		err = errors.Unwrap(err)
	}
	return strings.Join(msg, " -> ")
}
