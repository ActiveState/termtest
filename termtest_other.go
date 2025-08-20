//go:build !windows
// +build !windows

package termtest

import (
	"errors"
	"fmt"
)

func syscallErrorCode(err error) int {
	return -1
}

func (tt *TermTest) waitIndefinitely() error {
	tt.opts.Logger.Println("WaitIndefinitely called")
	defer tt.opts.Logger.Println("WaitIndefinitely closed")

	// Wait for listener to exit
	listenError := <-tt.listenError

	// Clean up pty
	tt.opts.Logger.Println("Closing pty")
	if err := tt.ptmx.Close(); err != nil {
		if syscallErrorCode(err) == 0 {
			tt.opts.Logger.Println("Ignoring 'The operation completed successfully' error")
		} else if errors.Is(err, ERR_ACCESS_DENIED) {
			// Ignore access denied error - means process has already finished
			tt.opts.Logger.Println("Ignoring access denied error")
		} else {
			return errors.Join(listenError, fmt.Errorf("failed to close pty: %w", err))
		}
	}
	tt.opts.Logger.Println("Closed pty")

	return listenError
}
