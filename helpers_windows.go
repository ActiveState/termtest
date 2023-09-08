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
func cleanPtySnapshot(snapshot []byte, isPosix bool) []byte {
	if isPosix {
		return snapshot
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

	var result []rune
	runes := bytes.Runes(snapshot)
	for _, r := range runes {
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
			continue

		// Detect start of complex escape sequence
		case inEscapeSequence && !inTitleEscapeSequence && (escapeSequenceCode == "0" || escapeSequenceCode == "2"):
			inTitleEscapeSequence = true
			recordingCode = false
			continue

		// SEQUENCE END

		// Detect end of escape sequence
		case inEscapeSequence && !inTitleEscapeSequence && bytes.ContainsRune(plainVirtualEscapeSeqEndValues, r):
			inEscapeSequence = false
			continue

		// Detect end of complex escape sequence
		case inTitleEscapeSequence && r == UnicodeBellRune:
			inEscapeSequence = false
			inTitleEscapeSequence = false
			continue

		// SEQUENCE CONTINUATION

		case inEscapeSequence && recordingCode:
			if r == ']' {
				continue
			}
			if !bytes.ContainsRune(numbers, r) {
				recordingCode = false
				continue
			}
			escapeSequenceCode += string(r)

		// Detect continuation of escape sequence
		case inEscapeSequence:
			recordingCode = false
			continue

		// OUTSIDE OF ESCAPE SEQUENCE

		case r == UnicodeBackspaceRune && len(result) > 0:
			result = result[:len(result)-1]

		default:
			result = append(result, r)
		}
	}
	return []byte(string(result))
}
