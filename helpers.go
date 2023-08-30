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

// cleanPtySnapshotWindows removes virtual escape sequences from the given byte slice
// https://learn.microsoft.com/en-us/windows/console/console-virtual-terminal-sequences
func cleanPtySnapshotWindows(b []byte) (o []byte) {
	// All escape sequences appear to end on `A-Za-z@`
	virtualEscapeSeqEndValues := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz@")
	inEscapeSequence := false

	return bytes.Map(func(r rune) rune {
		switch {
		// Detect start of sequence
		case !inEscapeSequence && r == 27:
			inEscapeSequence = true
			return -1

		// Detect end of sequence
		case inEscapeSequence && bytes.ContainsRune(virtualEscapeSeqEndValues, r):
			inEscapeSequence = false
			return -1

		// Anything between start and end of escape sequence should also be dropped
		case inEscapeSequence:
			return -1

		default:
			return r
		}
	}, b)
}

func reverse[S ~[]E, E any](s S) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func NormalizeLineEnds(v string) string {
	return strings.ReplaceAll(v, lineSepWindows, lineSepPosix)
}

func NormalizeLineEndsB(v []byte) []byte {
	return bytes.ReplaceAll(v, []byte(lineSepWindows), []byte(lineSepPosix))
}
