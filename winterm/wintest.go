// +build windows

package winterm

import (
	"syscall"
)


func GetStdoutConsoleMode() (int, error) {
	stdOutHandle, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil {
		return 0, err
	}

	mode, err := GetConsoleMode(uintptr(stdOutHandle))
	if err != nil {
		return 0, err
	}
	return int(mode), err
}