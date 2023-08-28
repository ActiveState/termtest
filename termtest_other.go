//go:build !windows
// +build !windows

package termtest

func syscallErrorCode(err error) int {
	return -1
}
