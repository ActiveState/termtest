package termtest

import (
	"errors"
	"fmt"
	"syscall"
	"time"

	gopsutil "github.com/shirou/gopsutil/v3/process"
)

func syscallErrorCode(err error) int {
	if errv, ok := err.(syscall.Errno); ok {
		return int(errv)
	}
	return 0
}

// WaitIndefinitely on Windows has to work around a Windows PTY bug where the PTY will NEVER exit by itself:
// https://github.com/photostorm/pty/issues/3
// Instead we wait for the process itself to exit, and after a grace period will shut down the pty.
func (tt *TermTest) WaitIndefinitely() error {
	tt.opts.Logger.Println("WaitIndefinitely called")
	defer tt.opts.Logger.Println("WaitIndefinitely closed")

	var procErr error

	tt.opts.Logger.Printf("Waiting for PID %d to exit\n", tt.Cmd().Process.Pid)
	for {
		// There is a race condition here; which is that the pty could still be processing the last of the output
		// when the process exits. This sleep tries to work around this, but on slow hosts this may not be sufficient.
		// This also gives some time in between process lookups
		time.Sleep(100 * time.Millisecond)

		// For some reason os.Process will always return a process even when the process has exited.
		// According to the docs this shouldn't happen, but here we are.
		// Using gopsutil seems to correctly identify the (not) running process.
		exists, err := gopsutil.PidExists(int32(tt.Cmd().Process.Pid))
		if err != nil {
			return fmt.Errorf("could not find process: %d: %w", tt.Cmd().Process.Pid, err)
		}
		if !exists {
			break
		}
	}

	// Clean up pty
	tt.opts.Logger.Println("Closing pty")
	if err := tt.ptmx.Close(); err != nil {
		if syscallErrorCode(err) == 0 {
			tt.opts.Logger.Println("Ignoring 'The operation completed successfully' error")
		} else if errors.Is(err, ERR_ACCESS_DENIED) {
			// Ignore access denied error - means process has already finished
			tt.opts.Logger.Println("Ignoring access denied error")
		} else {
			return errors.Join(procErr, fmt.Errorf("failed to close pty: %w", err))
		}
	}
	tt.opts.Logger.Println("Closed pty")

	// Now that the ptmx was closed the listener should also shut down
	return errors.Join(procErr, <-tt.listenError)
}
