//go:build windowsx
// +build windowsx

package termtest

func SyscallErrorCode(err error) int {
	return -1
}
