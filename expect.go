package termtest

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"
)

func (tt *TermTest) ExpectCustom(meets consumer, timeout time.Duration) error {
	return tt.outputDigester.addConsumer(meets, timeout).Wait()
}

// Expect listens to the terminal output and returns once the expected value is found or a timeout occurs
func (tt *TermTest) Expect(value string, timeout ...time.Duration) error {
	return tt.outputDigester.addConsumer(func(buffer string) (bool, error) {
		if strings.Contains(buffer, value) {
			return false, nil
		}
		return true, nil
	}, getIndex(timeout, 0, 10*time.Second)).Wait()
}

// ExpectRe listens to the terminal output and returns once the expected regular expression is matched or a timeout occurs
// Default timeout is 10 seconds
func (tt *TermTest) ExpectRe(rx regexp.Regexp, timeout ...time.Duration) error {
	return tt.outputDigester.addConsumer(func(buffer string) (keepListening bool, err error) {
		if rx.MatchString(buffer) {
			return false, nil
		}
		return true, nil
	}, getIndex(timeout, 0, 10*time.Second)).Wait()
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

func (tt *TermTest) expectExitCode(exitCode int, match bool, timeout ...time.Duration) error {
	timeoutV := getIndex(timeout, 0, 10*time.Second)
	timeoutC := time.After(timeoutV)
	for {
		select {
		case <-tt.closed:
			if tt.cmd.ProcessState != nil && tt.cmd.ProcessState.Exited() {
				if err := assertExitCode(tt.cmd.ProcessState.ExitCode(), exitCode, match); err != nil {
					return tt.opts.ExpectErrorHandler(tt, err)
				}
			}
			return tt.opts.ExpectErrorHandler(tt, fmt.Errorf("TermTest closed before ExpectExitCode was called"))
		case <-timeoutC:
			return tt.opts.ExpectErrorHandler(tt, fmt.Errorf("after %s: %w", timeoutV, TimeoutError))
		case exit := <-waitForCmdExit(tt.cmd):
			if exit.Err != nil {
				return tt.opts.ExpectErrorHandler(tt, exit.Err)
			}
			if err := assertExitCode(exit.ProcessState.ExitCode(), exitCode, match); err != nil {
				return tt.opts.ExpectErrorHandler(tt, err)
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
