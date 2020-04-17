// Copyright 2020 ActiveState Software. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file

package termtest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/ActiveState/vt10x"

	expect "github.com/ActiveState/go-expect"
	"github.com/ActiveState/termtest/internal/osutils"
)

var (
	// ErrNoProcess is returned when a process was expected to be running
	ErrNoProcess = errors.New("no command process seems to be running")
)

type errWaitTimeout struct {
	error
}

func (errWaitTimeout) Timeout() bool { return true }

// ConsoleProcess bonds a command with a pseudo-terminal for automation
type ConsoleProcess struct {
	opts    Options
	errs    chan error
	console *expect.Console
	vtstrip *vt10x.VTStrip
	cmd     *exec.Cmd
	cmdName string
	ctx     context.Context
	cancel  func()
}

// New bonds a command process with a console pty.
func New(opts Options) (*ConsoleProcess, error) {
	if err := opts.Normalize(); err != nil {
		return nil, err
	}

	cmd := exec.Command(opts.CmdName, opts.Args...)
	cmd.Dir = opts.WorkDirectory
	cmd.Env = opts.Environment

	// Create the process in a new process group.
	// This makes the behavior more consistent, as it isolates the signal handling from
	// the parent processes, which are dependent on the test environment.
	cmd.SysProcAttr = osutils.SysProcAttrForNewProcessGroup()
	fmt.Printf("Spawning '%s' from %s\n", osutils.CmdString(cmd), opts.WorkDirectory)

	expectObs := &expectObserverTransform{observeFn: opts.ObserveExpect}

	vtstrip := vt10x.NewStrip()

	console, err := expect.NewConsole(
		expect.WithDefaultTimeout(opts.DefaultTimeout),
		expect.WithReadBufferMutation(vtstrip.Strip),
		expect.WithSendObserver(expect.SendObserver(opts.ObserveSend)),
		expect.WithExpectObserver(expectObs.observe),
	)

	if err != nil {
		return nil, err
	}

	if err = console.Pty.StartProcessInTerminal(cmd); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	cp := ConsoleProcess{
		opts:    opts,
		errs:    make(chan error),
		console: console,
		vtstrip: vtstrip,
		cmd:     cmd,
		cmdName: opts.CmdName,
		ctx:     ctx,
		cancel:  cancel,
	}

	expectObs.setRawDataFn(cp.Snapshot)

	// Asynchronously wait for the underlying process to finish and communicate
	// results to `cp.errs` channel
	// Once the error has been received (by the `wait` function, the TTY is closed)
	go func() {
		defer close(cp.errs)

		err := cmd.Wait()

		select {
		case cp.errs <- err:
		case <-cp.ctx.Done():
			log.Println("ConsoleProcess cancelled!  You may have forgotten to call ExpectExitCode()")
			_ = console.Close()
			return
		}

		_ = console.CloseTTY()
	}()

	return &cp, nil
}

// Close cleans up all the resources allocated by the ConsoleProcess
// If the underlying process is still running, it is terminated with a SIGTERM signal.
func (cp *ConsoleProcess) Close() error {
	cp.cancel()

	_ = cp.opts.CleanUp()

	if cp.cmd == nil || cp.cmd.Process == nil {
		return nil
	}

	if cp.cmd.ProcessState != nil && cp.cmd.ProcessState.Exited() {
		return nil
	}

	if err := cp.cmd.Process.Kill(); err == nil {
		return nil
	}

	return cp.cmd.Process.Signal(syscall.SIGTERM)
}

// Executable returns the command name to be executed
func (cp *ConsoleProcess) Executable() string {
	return cp.cmdName
}

// WorkDirectory returns the directory in which the command shall be run
func (cp *ConsoleProcess) WorkDirectory() string {
	return cp.opts.WorkDirectory
}

// Snapshot returns a string containing a terminal snap-shot as a user would see it in a "real" terminal
func (cp *ConsoleProcess) Snapshot() string {
	return cp.console.Pty.State.String()
}

// TrimmedSnapshot displays the terminal output a user would see
// however the goroutine that creates this output is separate from this
// function so any output is not synced
func (cp *ConsoleProcess) TrimmedSnapshot() string {
	// When the PTY reaches 80 characters it continues output on a new line.
	// On Windows this means both a carriage return and a new line. Windows
	// also picks up any spaces at the end of the console output, hence all
	// the cleaning we must do here.
	newlineRe := regexp.MustCompile(`\r?\n`)
	return newlineRe.ReplaceAllString(strings.TrimSpace(cp.Snapshot()), "")
}

// ExpectRe listens to the terminal output and returns once the expected regular expression is matched or
// a timeout occurs
// Default timeout is 10 seconds
func (cp *ConsoleProcess) ExpectRe(value string, timeout ...time.Duration) {
	opts := []expect.ExpectOpt{expect.RegexpPattern(value)}
	if len(timeout) > 0 {
		opts = append(opts, expect.WithTimeout(timeout[0]))
	}

	cp.console.Expect(opts...)
}

// Expect listens to the terminal output and returns once the expected value is found or
// a timeout occurs
// Default timeout is 10 seconds
func (cp *ConsoleProcess) Expect(value string, timeout ...time.Duration) {
	opts := []expect.ExpectOpt{expect.String(value)}
	if len(timeout) > 0 {
		opts = append(opts, expect.WithTimeout(timeout[0]))
	}

	cp.console.Expect(opts...)
}

// WaitForInput returns once a shell prompt is active on the terminal
// Default timeout is 10 seconds
func (cp *ConsoleProcess) WaitForInput(timeout ...time.Duration) {
	usr, err := user.Current()
	if err != nil {
		panic(err)
	}

	msg := "echo wait_ready_$HOME"
	if runtime.GOOS == "windows" {
		msg = "echo wait_ready_%USERPROFILE%"
	}

	cp.SendLine(msg)
	cp.Expect("wait_ready_"+usr.HomeDir, timeout...)
}

// SendLine sends a new line to the terminal, as if a user typed it
func (cp *ConsoleProcess) SendLine(value string) {
	_, _ = cp.console.SendLine(value)
}

// Send sends a string to the terminal as if a user typed it
func (cp *ConsoleProcess) Send(value string) {
	_, _ = cp.console.Send(value)
}

// Signal sends an arbitrary signal to the running process
func (cp *ConsoleProcess) Signal(sig os.Signal) error {
	return cp.cmd.Process.Signal(sig)
}

// SendCtrlC tries to emulate what would happen in an interactive shell, when the user presses Ctrl-C
// Note: On Windows the Ctrl-C event is only reliable caught when the receiving process is
// listening for os.Interrupt signals.
func (cp *ConsoleProcess) SendCtrlC() {
	cp.Send(string([]byte{0x03})) // 0x03 is ASCII character for ^C
}

// Stop sends an interrupt signal for the tested process and fails if no process has been started yet.
// Note: This is not supported on Windows
func (cp *ConsoleProcess) Stop() error {
	if cp.cmd == nil || cp.cmd.Process == nil {
		return ErrNoProcess
	}
	return cp.cmd.Process.Signal(os.Interrupt)
}

type exitCodeMatcher struct {
	exitCode int
	expected bool
}

func (em *exitCodeMatcher) Match(_ interface{}) bool {
	return true
}

func (em *exitCodeMatcher) Criteria() interface{} {
	comparator := "=="
	if !em.expected {
		comparator = "!="
	}

	return fmt.Sprintf("exit code %s %d", comparator, em.exitCode)
}

// ExpectExitCode waits for the program under test to terminate, and checks that the returned exit code meets expectations
func (cp *ConsoleProcess) ExpectExitCode(exitCode int, timeout ...time.Duration) {
	_, buf, err := cp.wait(timeout...)
	if err == nil && exitCode == 0 {
		return
	}
	matchers := []expect.Matcher{&exitCodeMatcher{exitCode, true}}
	eexit, ok := err.(*exec.ExitError)
	if !ok {
		cp.opts.ObserveExpect(matchers, cp.TrimmedSnapshot(), buf, fmt.Errorf("process failed with error: %v", err))
		return
	}
	if eexit.ExitCode() != exitCode {
		cp.opts.ObserveExpect(matchers, cp.TrimmedSnapshot(), buf, fmt.Errorf("exit code wrong: was %d (expected %d)", eexit.ExitCode(), exitCode))
	}
}

// ExpectNotExitCode waits for the program under test to terminate, and checks that the returned exit code is not the value provide
func (cp *ConsoleProcess) ExpectNotExitCode(exitCode int, timeout ...time.Duration) {
	_, buf, err := cp.wait(timeout...)
	matchers := []expect.Matcher{&exitCodeMatcher{exitCode, false}}
	if err == nil {
		if exitCode == 0 {
			cp.opts.ObserveExpect(matchers, cp.TrimmedSnapshot(), buf, fmt.Errorf("exit code wrong: should not have been 0"))
		}
		return
	}
	eexit, ok := err.(*exec.ExitError)
	if !ok {
		cp.opts.ObserveExpect(matchers, cp.TrimmedSnapshot(), buf, fmt.Errorf("process failed with error: %v", err))
		return
	}
	if eexit.ExitCode() == exitCode {
		cp.opts.ObserveExpect(matchers, cp.TrimmedSnapshot(), buf, fmt.Errorf("exit code wrong: should not have been %d", exitCode))
	}
}

// Wait waits for the program under test to terminate, not caring about the exit code at all
func (cp *ConsoleProcess) Wait(timeout ...time.Duration) {
	_, _, err := cp.wait(timeout...)
	if err != nil {
		fmt.Printf("Process exited with error: %v (This is not fatal when using Wait())", err)
	}
}

// waitForEOF is a helper function that consumes all bytes until we reach an EOF signal
// and then closes up all the readers.
// This function is called as a last step by cp.wait()
func (cp *ConsoleProcess) waitForEOF(processErr error, deadline time.Time, buf *bytes.Buffer) (*os.ProcessState, string, error) {
	if time.Now().After(deadline) {
		return cp.cmd.ProcessState, buf.String(), &errWaitTimeout{fmt.Errorf("timeout waiting for exit code")}
	}
	b, expErr := cp.console.Expect(
		expect.OneOf(expect.PTSClosed, expect.StdinClosed, expect.EOF),
		expect.WithTimeout(deadline.Sub(time.Now())),
	)
	_, err := buf.WriteString(b)
	if err != nil {
		log.Printf("Failed to append to buffer: %v", err)
	}

	err = cp.console.CloseReaders()
	if err != nil {
		log.Printf("Failed to close the console readers: %v", err)
	}
	if expErr != nil {
		return nil, buf.String(), expErr
	}
	return cp.cmd.ProcessState, buf.String(), processErr
}

// forceKill kills the underlying process and waits until it return the exit error
func (cp *ConsoleProcess) forceKill() {
	if err := cp.cmd.Process.Kill(); err != nil {
		panic(err)
	}
	<-cp.errs
}

// wait waits for a console to finish and cleans up all resources
// First it consistently flushes/drains the pipe until the underlying process finishes.
// Note, that without draining the output pipe, the process might hang.
// As soon as the process actually finishes, it waits for the underlying console to be closed
// and gives all readers a chance to read remaining bytes.
func (cp *ConsoleProcess) wait(timeout ...time.Duration) (*os.ProcessState, string, error) {
	if cp.cmd == nil || cp.cmd.Process == nil {
		panic(ErrNoProcess.Error())
	}

	t := cp.opts.DefaultTimeout
	if len(timeout) > 0 {
		t = timeout[0]
	}

	deadline := time.Now().Add(t)
	buf := new(bytes.Buffer)
	// run in a tight loop until process finished or until we timeout
	for {
		err := cp.console.Drain(100*time.Millisecond, buf)

		if time.Now().After(deadline) {
			log.Println("killing process after timeout")
			cp.forceKill()
			return nil, buf.String(), &errWaitTimeout{fmt.Errorf("timeout waiting for exit code")}
		}
		// we only expect timeout or EOF errors here, otherwise we will kill the process
		if err != nil && !(os.IsTimeout(err) || err == io.EOF) {
			log.Printf("killing process after unknown error: %v\n", err)
			cp.forceKill()
			return nil, buf.String(), fmt.Errorf("unexpected error while waiting for exit code: %v", err)

		}

		select {
		case perr := <-cp.errs:
			return cp.waitForEOF(perr, deadline, buf)
		case <-cp.ctx.Done():
			return nil, buf.String(), fmt.Errorf("ConsoleProcess context canceled")
		default:
		}
	}
}
