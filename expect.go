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

type ExpectOpts struct {
	ExpectTimeout bool
	Timeout       time.Duration
	ErrorHandler  ErrorHandler

	// Sends the full buffer each time, with the latest data appended to the end.
	// This is the full buffer as of the point in time that the consumer started listening.
	SendFullBuffer bool
}

func NewExpectOpts(opts ...SetExpectOpt) (*ExpectOpts, error) {
	o := &ExpectOpts{}
	for _, opt := range opts {
		if err := opt(o); err != nil {
			return nil, err
		}
	}
	return o, nil
}

func (o *ExpectOpts) ToConsumerOpts() []SetConsOpt {
	var consOpts []SetConsOpt
	if o.SendFullBuffer {
		consOpts = append(consOpts, OptConsSendFullBuffer())
	}
	if o.Timeout > 0 {
		consOpts = append(consOpts)
	}

	return consOpts
}

type SetExpectOpt func(o *ExpectOpts) error

func SetTimeout(timeout time.Duration) SetExpectOpt {
	return func(o *ExpectOpts) error {
		o.Timeout = timeout
		return nil
	}
}

func SetSendFullBuffer() SetExpectOpt {
	return func(o *ExpectOpts) error {
		o.SendFullBuffer = true
		return nil
	}
}

func SetErrorHandler(handler ErrorHandler) SetExpectOpt {
	return func(o *ExpectOpts) error {
		o.ErrorHandler = handler
		return nil
	}
}

func (tt *TermTest) expectErrorHandler(rerr *error, opts *ExpectOpts) {
	err := *rerr
	if err == nil {
		return
	}

	// Sanitize error messages so we can easily interpret the results
	switch {
	case errors.Is(err, StopPrematureError):
		err = &ExpectNotMetDueToStopError{err}
	}

	errorHandler := tt.opts.ExpectErrorHandler
	if opts.ErrorHandler != nil {
		errorHandler = opts.ErrorHandler
	}

	*rerr = errorHandler(tt, err)
	return
}

func (tt *TermTest) ExpectCustom(consumer consumer, opts ...SetExpectOpt) (rerr error) {
	expectOpts, err := NewExpectOpts(opts...)
	defer tt.expectErrorHandler(&rerr, expectOpts)
	if err != nil {
		return fmt.Errorf("could not create expect options: %w", err)
	}

	cons, err := tt.outputProducer.addConsumer(consumer, expectOpts.ToConsumerOpts()...)
	if err != nil {
		return fmt.Errorf("could not add consumer: %w", err)
	}
	return cons.wait()
}

// Expect listens to the terminal output and returns once the expected value is found or a timeout occurs
func (tt *TermTest) Expect(value string, opts ...SetExpectOpt) error {
	return tt.ExpectCustom(func(buffer string) (bool, error) {
		return expect(value, buffer)
	}, append([]SetExpectOpt{SetSendFullBuffer()}, opts...)...)
}

func expect(value, buffer string) (bool, error) {
	return strings.Contains(buffer, value), nil
}

// ExpectRe listens to the terminal output and returns once the expected regular expression is matched or a timeout occurs
// Default timeout is 10 seconds
func (tt *TermTest) ExpectRe(rx regexp.Regexp, opts ...SetExpectOpt) error {
	return tt.ExpectCustom(func(buffer string) (bool, error) {
		return expectRe(rx, buffer)
	}, append([]SetExpectOpt{SetSendFullBuffer()}, opts...)...)
}

func expectRe(rx regexp.Regexp, buffer string) (bool, error) {
	return rx.MatchString(buffer), nil
}

// ExpectInput returns once a shell prompt is active on the terminal
func (tt *TermTest) ExpectInput(opts ...SetExpectOpt) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not get user home directory: %w", err)
	}

	msg := "echo wait_ready_$HOME"
	if runtime.GOOS == "windows" {
		msg = "echo wait_ready_%USERPROFILE%"
	}

	if err := tt.SendLine(msg); err != nil {
		return fmt.Errorf("could not send line to terminal: %w", err)
	}
	return tt.Expect("wait_ready_"+homeDir, opts...)
}

// ExpectExitCode waits for the program under test to terminate, and checks that the returned exit code meets expectations
func (tt *TermTest) ExpectExitCode(exitCode int, opts ...SetExpectOpt) error {
	return tt.expectExitCode(exitCode, true, opts...)
}

// ExpectNotExitCode waits for the program under test to terminate, and checks that the returned exit code is not the value provide
func (tt *TermTest) ExpectNotExitCode(exitCode int, opts ...SetExpectOpt) error {
	return tt.expectExitCode(exitCode, false, opts...)
}

// ExpectExit waits for the program under test to terminate, not caring about the exit code
func (tt *TermTest) ExpectExit(opts ...SetExpectOpt) error {
	return tt.expectExitCode(-999, false, opts...)
}

func (tt *TermTest) expectExitCode(exitCode int, match bool, opts ...SetExpectOpt) (rerr error) {
	expectOpts, err := NewExpectOpts(opts...)
	defer tt.expectErrorHandler(&rerr, expectOpts)
	if err != nil {
		return fmt.Errorf("could not create expect options: %w", err)
	}

	timeoutV := 5 * time.Second
	if expectOpts.Timeout > 0 {
		timeoutV = expectOpts.Timeout
	}
	timeoutC := time.After(timeoutV)
	for {
		select {
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
