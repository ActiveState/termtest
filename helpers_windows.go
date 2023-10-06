package termtest

import (
	"bytes"

	"golang.org/x/sys/windows"
)

var ERR_ACCESS_DENIED = windows.ERROR_ACCESS_DENIED

const UnicodeEscapeRune = '\u001B'
const UnicodeBellRune = '\u0007'
const UnicodeBackspaceRune = '\u0008' // Note in the docs this is \u007f, but in actual use we're seeing \u0008. Possibly badly documented.

// cleanPtySnapshot removes windows console escape sequences from the output so we can interpret it plainly.
// Ultimately we want to emulate the windows console here, just like we're doing for v10x on posix.
// The current implementation is geared towards our needs, and won't be able to handle all escape sequences as a result.
// For details on escape sequences see https://learn.microsoft.com/en-us/windows/console/console-virtual-terminal-sequences
func cleanPtySnapshot(snapshot []byte, cursorPos int, isPosix bool) ([]byte, int) {
	if isPosix {
		return snapshot, cursorPos
	}

	// Most escape sequences appear to end on `A-Za-z@`
	plainVirtualEscapeSeqEndValues := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz@")

	// Cheaper than converting to ints
	numbers := []byte("0123456789")

	// Some escape sequences are more complex, such as window titles
	recordingCode := false
	escapeSequenceCode := ""
	inEscapeSequence := false
	inTitleEscapeSequence := false

	newCursorPos := cursorPos
	dropPos := func(pos int) {
		if pos <= cursorPos {
			newCursorPos--
		}
	}

	var result []rune
	runes := bytes.Runes(snapshot)
	for pos, r := range runes {
		// Reset code recording outside of escape sequence, so we don't have to manually handle this throughout
		if !inEscapeSequence {
			recordingCode = false
			escapeSequenceCode = ""
		}
		switch {
		// SEQUENCE START

		// Detect start of escape sequence
		case !inEscapeSequence && r == UnicodeEscapeRune:
			inEscapeSequence = true
			recordingCode = true
			dropPos(pos)
			continue

		// Detect start of complex escape sequence
		case inEscapeSequence && !inTitleEscapeSequence && (escapeSequenceCode == "0" || escapeSequenceCode == "2"):
			inTitleEscapeSequence = true
			recordingCode = false
			dropPos(pos)
			continue

		// SEQUENCE END

		// Detect end of escape sequence
		case inEscapeSequence && !inTitleEscapeSequence && bytes.ContainsRune(plainVirtualEscapeSeqEndValues, r):
			inEscapeSequence = false
			dropPos(pos)
			continue

		// Detect end of complex escape sequence
		case inTitleEscapeSequence && r == UnicodeBellRune:
			inEscapeSequence = false
			inTitleEscapeSequence = false
			dropPos(pos)
			continue

		// SEQUENCE CONTINUATION

		case inEscapeSequence && recordingCode && bytes.ContainsRune(numbers, r):
			escapeSequenceCode += string(r)
			dropPos(pos)
			continue

		// Detect continuation of escape sequence
		case inEscapeSequence:
			if r != ']' {
				recordingCode = false
			}
			dropPos(pos)
			continue

		// OUTSIDE OF ESCAPE SEQUENCE

		case r == UnicodeBackspaceRune && len(result) > 0:
			dropPos(pos - 1)
			dropPos(pos)
			result = result[:len(result)-1]
			continue

		default:
			result = append(result, r)
		}
	}
	return []byte(string(result)), newCursorPos
}
