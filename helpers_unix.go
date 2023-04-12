//go:build !windows
// +build !windows

package termtest

import (
	"bytes"
	"errors"
)

var ERR_ACCESS_DENIED = errors.New("only used on windows, this should never match")

func cleanPtySnapshot(b []byte, _ bool) []byte {
	return bytes.TrimRight(b, "\x00")
}
