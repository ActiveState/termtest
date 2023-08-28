package termtest

import "syscall"

func syscallErrorCode(err error) int {
	if errv, ok := err.(syscall.Errno); ok {
		return int(errv)
	}
	return 0
}
