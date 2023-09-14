package termtest

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

type voidWriter struct{}

func (v voidWriter) Write(p []byte) (n int, err error) { return len(p), nil }

var neverGonnaHappen = time.Hour * 24 * 365 * 100

var lineSepPosix = "\n"
var lineSepWindows = "\r\n"

type cmdExit struct {
	ProcessState *os.ProcessState
	Err          error
}

// waitForCmdExit turns process.wait() into a channel so that it can be used within a select{} statement
func waitForCmdExit(cmd *exec.Cmd) chan cmdExit {
	exit := make(chan cmdExit, 1)
	go func() {
		err := cmd.Wait()
		exit <- cmdExit{ProcessState: cmd.ProcessState, Err: err}
	}()
	return exit
}

func waitChan[T any](wait func() T) chan T {
	done := make(chan T)
	go func() {
		wait()
		close(done)
	}()
	return done
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

	// Reverse the slice so that the most inner error is first
	reverse(msg)

	return strings.Join(msg, " -> ")
}

func reverse[S ~[]E, E any](s S) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func NormalizeLineEnds(v string) string {
	return strings.ReplaceAll(v, "\r", "")
}

func NormalizeLineEndsB(v []byte) []byte {
	return bytes.ReplaceAll(v, []byte("\r"), []byte(""))
}

func copyBytes(b []byte) []byte {
	return append([]byte{}, b...)
}
