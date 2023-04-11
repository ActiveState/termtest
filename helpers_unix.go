//go:build !windows
// +build !windows

package termtest

import (
	"errors"
)

var ERR_ACCESS_DENIED = errors.New("only used on windows, this should never match")
