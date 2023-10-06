package termtest

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type ExpectOpts struct {
	ExpectTimeout bool
	Timeout       time.Duration
	ErrorHandler  ErrorHandler
	ErrorMessage  string
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
	if o.Timeout > 0 {
		consOpts = append(consOpts, OptsConsTimeout(o.Timeout))
	}

	return consOpts
}

type SetExpectOpt func(o *ExpectOpts) error

func OptExpectErrorMessage(msg string) SetExpectOpt {
	return func(o *ExpectOpts) error {
		o.ErrorMessage = msg
		return nil
	}
}

func OptExpectTimeout(timeout time.Duration) SetExpectOpt {
	return func(o *ExpectOpts) error {
		o.Timeout = timeout
		return nil
	}
}

func OptExpectErrorHandler(handler ErrorHandler) SetExpectOpt {
	return func(o *ExpectOpts) error {
		o.ErrorHandler = handler
		return nil
	}
}

func OptExpectSilenceErrorHandler() SetExpectOpt {
	return func(o *ExpectOpts) error {
		o.ErrorHandler = SilenceErrorHandler()
		return nil
	}
}

func (tt *TermTest) ExpectErrorHandler(rerr *error, opts *ExpectOpts) error {
	err := *rerr
	if err == nil {
		return nil
	}

	errorHandler := tt.opts.ExpectErrorHandler
	if opts.ErrorHandler != nil {
		errorHandler = opts.ErrorHandler
	}

	*rerr = errorHandler(tt, err)
	return *rerr
}

func (tt *TermTest) ExpectCustom(consumer consumer, opts ...SetExpectOpt) (rerr error) {
	opts = append([]SetExpectOpt{OptExpectTimeout(tt.opts.DefaultTimeout)}, opts...)
	expectOpts, err := NewExpectOpts(opts...)
	defer tt.ExpectErrorHandler(&rerr, expectOpts)
	if err != nil {
		return fmt.Errorf("could not create expect options: %w", err)
	}

	cons, err := tt.outputProducer.addConsumer(consumer, expectOpts.ToConsumerOpts()...)
	if err != nil {
		return fmt.Errorf("could not add consumer: %w", err)
	}
	err = cons.wait()
	if err != nil && expectOpts.ErrorMessage != "" {
		return fmt.Errorf("%s: %w", expectOpts.ErrorMessage, err)
	}
	return err
}

// Expect listens to the terminal output and returns once the expected value is found or a timeout occurs
func (tt *TermTest) Expect(value string, opts ...SetExpectOpt) error {
	tt.opts.Logger.Printf("Expect: %#v\n", value)

	return tt.ExpectCustom(func(buffer string) (int, error) {
		return tt.expect(value, buffer)
	}, append([]SetExpectOpt{OptExpectErrorMessage(fmt.Sprintf("Expected: %#v", value))}, opts...)...)
}

func (tt *TermTest) expect(value, buffer string) (endPos int, rerr error) {
	if tt.opts.NormalizedLineEnds {
		tt.opts.Logger.Println("NormalizedLineEnds prior to expect")
		value = NormalizeLineEnds(value)
	}

	tt.opts.Logger.Printf("expect: '%#v', buffer: '%#v'\n", string(value), strings.Trim(strings.TrimSpace(buffer), "\x00"))
	defer func() {
		tt.opts.Logger.Printf("Match: %v\n", endPos > 0)
	}()
	idx := strings.Index(buffer, value)
	if idx == -1 {
		return 0, nil
	}
	return idx + len(value), nil
}

// ExpectRe listens to the terminal output and returns once the expected regular expression is matched or a timeout occurs
// Default timeout is 10 seconds
func (tt *TermTest) ExpectRe(rx *regexp.Regexp, opts ...SetExpectOpt) error {
	tt.opts.Logger.Printf("ExpectRe: %s\n", rx.String())

	return tt.ExpectCustom(func(buffer string) (int, error) {
		return expectRe(rx, buffer)
	}, append([]SetExpectOpt{OptExpectErrorMessage(fmt.Sprintf("Expected Regex: %#v", rx.String()))}, opts...)...)
}

func expectRe(rx *regexp.Regexp, buffer string) (int, error) {
	idx := rx.FindIndex([]byte(buffer))
	if idx == nil {
		return 0, nil
	}
	return idx[1], nil
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
	tt.opts.Logger.Printf("Expecting exit code %d: %v", exitCode, match)
	defer func() {
		tt.opts.Logger.Printf("Expect exit code result: %s", unwrapErrorMessage(rerr))
	}()

	expectOpts, err := NewExpectOpts(opts...)
	defer tt.ExpectErrorHandler(&rerr, expectOpts)
	if err != nil {
		return fmt.Errorf("could not create expect options: %w", err)
	}

	timeoutV := tt.opts.DefaultTimeout
	if expectOpts.Timeout > 0 {
		timeoutV = expectOpts.Timeout
	}

	timeoutTotal := time.Now().Add(timeoutV)

	// While Wait() below will wait for the cmd exit, we want to call it here separately because to us cmd.Wait() can
	// return an error and still be valid, whereas Wait() would interrupt if it reached that point.
	select {
	case <-time.After(timeoutV):
		return fmt.Errorf("after %s: %w", timeoutV, TimeoutError)
	case err := <-waitChan(tt.cmd.Wait):
		if err != nil && (tt.cmd.ProcessState == nil || tt.cmd.ProcessState.ExitCode() == 0) {
			return fmt.Errorf("cmd wait failed: %w", err)
		}
		if err := tt.assertExitCode(tt.cmd.ProcessState.ExitCode(), exitCode, match); err != nil {
			return err
		}
	}

	if err := tt.Wait(timeoutTotal.Sub(time.Now())); err != nil {
		return fmt.Errorf("wait failed: %w", err)
	}

	return nil
}

func (tt *TermTest) assertExitCode(exitCode, comparable int, match bool) error {
	tt.opts.Logger.Printf("assertExitCode: exitCode=%d, comparable=%d, match=%v\n", exitCode, comparable, match)
	if compared := exitCode == comparable; compared != match {
		if match {
			return fmt.Errorf("expected exit code %d, got %d", comparable, exitCode)
		} else {
			return fmt.Errorf("expected exit code to not be %d", exitCode)
		}
	}
	return nil
}
