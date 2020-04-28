// +build !windows

package winterm

func GetStdoutConsoleMode() (int, error) {
	return 0, nil
}