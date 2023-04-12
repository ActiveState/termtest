package termtest

import (
	"bytes"

	"golang.org/x/sys/windows"
)

var ERR_ACCESS_DENIED = windows.ERROR_ACCESS_DENIED

func cleanPtySnapshot(b []byte, isPosix bool) []byte {
	b = bytes.TrimRight(b, "\x00")

	if isPosix {
		return b
	}

	// If non-posix we need to remove virtual escape sequences from the given byte slice
	// https://learn.microsoft.com/en-us/windows/console/console-virtual-terminal-sequences

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
