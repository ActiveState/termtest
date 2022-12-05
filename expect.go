package termtest

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"
)

type ExpectNotMetDueToStopError struct {
	err error
}

func (e *ExpectNotMetDueToStopError) Error() string {
	return "expectation not met by the time the process finished"
}

func (e *ExpectNotMetDueToStopError) Unwrap() error {
	return e.err
}

func errorHandler(tt *TermTest, rerr *error) {
	err := *rerr
	if err == nil {
		return
	}

	// Sanitize error messages so we can easily interpret the results
	switch {
	case errors.Is(err, StopPrematureError):
		err = &ExpectNotMetDueToStopError{err}
	}

	*rerr = tt.opts.ExpectErrorHandler(tt, err)
	return
}

func (tt *TermTest) ExpectCustom(consumer consumer, timeout time.Duration, opts ...SetConsOpt) (rerr error) {
	defer errorHandler(tt, &rerr)
	return tt.outputDigester.addConsumer(consumer, timeout, opts...).Wait()
}

// Expect listens to the terminal output and returns once the expected value is found or a timeout occurs
func (tt *TermTest) Expect(value string, timeout ...time.Duration) error {
	return tt.ExpectCustom(func(buffer string) (bool, error) {
		return expect(value, buffer)
	}, getIndex(timeout, 0, 10*time.Second), OptSendFullBuffer())
}

func expect(value, buffer string) (bool, error) {
	return strings.Contains(buffer, value), nil
}

// ExpectRe listens to the terminal output and returns once the expected regular expression is matched or a timeout occurs
// Default timeout is 10 seconds
func (tt *TermTest) ExpectRe(rx regexp.Regexp, timeout ...time.Duration) error {
	return tt.ExpectCustom(func(buffer string) (bool, error) {
		return expectRe(rx, buffer)
	}, getIndex(timeout, 0, 10*time.Second), OptSendFullBuffer())
}

func expectRe(rx regexp.Regexp, buffer string) (bool, error) {
	return rx.MatchString(buffer), nil
}

// ExpectInput returns once a shell prompt is active on the terminal
func (tt *TermTest) ExpectInput(timeout ...time.Duration) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not get user home directory: %w", err)
	}

	msg := "echo wait_ready_$HOME"
	if runtime.GOOS == "windows" {
		msg = "echo wait_ready_%USERPROFILE%"
	}

	tt.SendLine(msg)
	return tt.Expect("wait_ready_"+homeDir, timeout...)
}

// ExpectExitCode waits for the program under test to terminate, and checks that the returned exit code meets expectations
func (tt *TermTest) ExpectExitCode(exitCode int, timeout ...time.Duration) error {
	return tt.expectExitCode(exitCode, true, timeout...)
}

// ExpectNotExitCode waits for the program under test to terminate, and checks that the returned exit code is not the value provide
func (tt *TermTest) ExpectNotExitCode(exitCode int, timeout ...time.Duration) error {
	return tt.expectExitCode(exitCode, false, timeout...)
}

// ExpectExit waits for the program under test to terminate, not caring about the exit code
func (tt *TermTest) ExpectExit(timeout ...time.Duration) error {
	return tt.expectExitCode(-999, false, timeout...)
}

func (tt *TermTest) expectExitCode(exitCode int, match bool, timeout ...time.Duration) (rerr error) {
	defer errorHandler(tt, &rerr)

	timeoutV := getIndex(timeout, 0, 10*time.Second)
	timeoutC := time.After(timeoutV)
	for {
		select {
		case <-tt.closed:
			return fmt.Errorf("TermTest closed before ExpectExitCode was called")
		case <-timeoutC:
			return fmt.Errorf("after %s: %w", timeoutV, TimeoutError)
		case exit := <-waitForCmdExit(tt.cmd):
			if exit.Err != nil {
				return exit.Err
			}
			if err := assertExitCode(exit.ProcessState.ExitCode(), exitCode, match); err != nil {
				return err
			}
			return nil // Expectation met
		}
	}
}

func assertExitCode(exitCode, comparable int, match bool) error {
	if compared := exitCode == comparable; compared != match {
		if match {
			return fmt.Errorf("expected exit code %d, got %d", comparable, exitCode)
		} else {
			return fmt.Errorf("expected exit code to not be %d", exitCode)
		}
	}
	return nil
}
