// Copyright 2018 Netflix, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package expect

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"runtime"
	"time"
	"unicode/utf8"

	"github.com/ActiveState/go-expect/internal/osutils"
	"github.com/ActiveState/xpty"
)

// Console is an interface to automate input and output for interactive
// applications. Console can block until a specified output is received and send
// input back on it's tty. Console can also multiplex other sources of input
// and multiplex its output to other writers.
type Console struct {
	opts            ConsoleOpts
	Pty             *xpty.Xpty
	passthroughPipe *PassthroughPipe
	runeReader      *bufio.Reader
	closers         []io.Closer
}

// ConsoleOpt allows setting Console options.
type ConsoleOpt func(*ConsoleOpts) error

// ConsoleOpts provides additional options on creating a Console.
type ConsoleOpts struct {
	Logger          *log.Logger
	Stdins          []io.Reader
	Stdouts         []io.Writer
	Closers         []io.Closer
	ExpectObservers []ExpectObserver
	SendObservers   []SendObserver
	ReadTimeout     *time.Duration
	ReadBufMutation func([]byte) ([]byte, error)
}

// ExpectObserver provides an interface for a function callback that will
// be called after each Expect operation.
// matchers will be the list of active matchers when an error occurred,
//   or a list of matchers that matched `buf` when err is nil.
// buf is the captured output that was matched against.
// err is error that might have occurred. May be nil.
type ExpectObserver func(matchers []Matcher, buf string, err error)

// SendObserver provides an interface for a function callback that will
// be called after each Send operation.
// msg is the string that was sent.
// num is the number of bytes actually sent.
// err is the error that might have occured.  May be nil.
type SendObserver func(msg string, num int, err error)

// WithStdout adds writers that Console duplicates writes to, similar to the
// Unix tee(1) command.
//
// Each write is written to each listed writer, one at a time. Console is the
// last writer, writing to it's internal buffer for matching expects.
// If a listed writer returns an error, that overall write operation stops and
// returns the error; it does not continue down the list.
func WithStdout(writers ...io.Writer) ConsoleOpt {
	return func(opts *ConsoleOpts) error {
		opts.Stdouts = append(opts.Stdouts, writers...)
		return nil
	}
}

// WithStdin adds readers that bytes read are written to Console's  tty. If a
// listed reader returns an error, that reader will not be continued to read.
func WithStdin(readers ...io.Reader) ConsoleOpt {
	return func(opts *ConsoleOpts) error {
		opts.Stdins = append(opts.Stdins, readers...)
		return nil
	}
}

// WithCloser adds closers that are closed in order when Console is closed.
func WithCloser(closer ...io.Closer) ConsoleOpt {
	return func(opts *ConsoleOpts) error {
		opts.Closers = append(opts.Closers, closer...)
		return nil
	}
}

// WithLogger adds a logger for Console to log debugging information to. By
// default Console will discard logs.
func WithLogger(logger *log.Logger) ConsoleOpt {
	return func(opts *ConsoleOpts) error {
		opts.Logger = logger
		return nil
	}
}

// WithExpectObserver adds an ExpectObserver to allow monitoring Expect operations.
func WithExpectObserver(observers ...ExpectObserver) ConsoleOpt {
	return func(opts *ConsoleOpts) error {
		opts.ExpectObservers = append(opts.ExpectObservers, observers...)
		return nil
	}
}

// WithSendObserver adds a SendObserver to allow monitoring Send operations.
func WithSendObserver(observers ...SendObserver) ConsoleOpt {
	return func(opts *ConsoleOpts) error {
		opts.SendObservers = append(opts.SendObservers, observers...)
		return nil
	}
}

// WithDefaultTimeout sets a default read timeout during Expect statements.
func WithDefaultTimeout(timeout time.Duration) ConsoleOpt {
	return func(opts *ConsoleOpts) error {
		opts.ReadTimeout = &timeout
		return nil
	}
}

// WithReadBufferMutation sets a transformation function to prepare console
// reads.
func WithReadBufferMutation(fn func([]byte) ([]byte, error)) ConsoleOpt {
	return func(opts *ConsoleOpts) error {
		opts.ReadBufMutation = fn
		return nil
	}
}

// NewConsole returns a new Console with the given options.
func NewConsole(opts ...ConsoleOpt) (*Console, error) {
	options := ConsoleOpts{
		Logger:          log.New(ioutil.Discard, "", 0),
		ReadBufMutation: func(bs []byte) ([]byte, error) { return bs, nil },
	}

	for _, opt := range opts {
		if err := opt(&options); err != nil {
			return nil, err
		}
	}

	var pty *xpty.Xpty
	// On Windows we are adding an extra row, because the last row appears to be empty usually
	rows := uint16(30)
	if runtime.GOOS == "windows" {
		rows = 31
	}
	pty, err := xpty.New(80, rows)
	if err != nil {
		return nil, err
	}
	closers := append(options.Closers)

	c := &Console{
		opts: options,
		Pty:  pty,
	}
	passthroughPipe := NewPassthroughPipe(c)

	closers = append(options.Closers, passthroughPipe)
	c.passthroughPipe = passthroughPipe

	// We provide a generous 10kB buffer for the rune-reader, because when the buffer is full,
	// the writer gets blocked.  In our case the writer is ultimately the terminal output pipe,
	// ie., the pseudo-terminal.  It seems OS dependent on how the pseudo-terminal reacts when
	// it cannot write anymore:
	// On Linux and Windows, bytes seem to be discarded, whereas MacOS just blocks the entire process.
	// If expected strings cannot be matched because of missing bytes, consider increasing the buffer
	// size even more, or switching to a `bytes.Buffer`.
	c.runeReader = bufio.NewReaderSize(passthroughPipe, 10*1024)
	c.closers = closers

	for _, stdin := range options.Stdins {
		go func(stdin io.Reader) {
			_, err := io.Copy(c, stdin)
			if err != nil {
				c.Logf("failed to copy stdin: %s", err)
			}
		}(stdin)
	}

	return c, nil
}

// Tty returns Console's pts (slave part of a pty). A pseudoterminal, or pty is
// a pair of psuedo-devices, one of which, the slave, emulates a real text
// terminal device.
func (c *Console) Tty() *os.File {
	return c.Pty.Tty()
}

// Drain reads from the input stream until it catches up with the incoming stream off data.
// This function can unblock the writer, if no further reads from the passthrough pipe are
// needed
func (c *Console) Drain(t time.Duration, buf *bytes.Buffer) error {
	writer := io.MultiWriter(append(c.opts.Stdouts, buf)...)
	runeWriter := bufio.NewWriterSize(writer, utf8.UTFMax)

	c.passthroughPipe.SetReadDeadline(time.Now().Add(t))
	for {
		r, _, err := c.runeReader.ReadRune()
		if err != nil {
			return err
		}
		_, err = runeWriter.WriteRune(r)
		if err != nil {
			return err
		}

		// Immediately flush rune to the underlying writers.
		err = runeWriter.Flush()
		if err != nil {
			return err
		}
	}
}

// Read reads bytes b from Console's tty.
func (c *Console) Read(b []byte) (int, error) {
	n, err := c.Pty.TerminalOutPipe().Read(b)
	if err != nil {
		return n, err
	}

	bs, err := c.opts.ReadBufMutation(b[:n])
	copy(b[0:len(bs)], bs)
	return len(bs), err
}

// Write writes bytes b to Console's tty.
func (c *Console) Write(b []byte) (int, error) {
	c.Logf("console write: %q", b)
	return c.Pty.TerminalInPipe().Write(b)
}

// Fd returns Console's file descripting referencing the master part of its
// pty.
func (c *Console) Fd() uintptr {
	return c.Pty.TerminalOutFd()
}

// CloseTTY closes Console's tty. Calling CloseTTY will unblock Expect and ExpectEOF.
func (c *Console) CloseTTY() error {
	// close the tty in the end
	return c.Pty.Close()
}

func (c *Console) CloseReaders() error {
	for _, fd := range c.closers {
		err := fd.Close()
		if err != nil {
			c.Logf("failed to close: %s", err)
		}
	}

	return c.passthroughPipe.Close()
}

// Close closes both the TTY and afterwards all the readers
// You may want to split this up to give the readers time to read all the data
// until they reach the EOF error
func (c *Console) Close() error {
	err := c.CloseTTY()
	if err != nil {
		c.Logf("failed to close TTY: %v", err)
	}

	// close the readers reading from the TTY
	return c.CloseReaders()
}

// Send writes string s to Console's tty.
func (c *Console) Send(s string) (int, error) {
	c.Logf("console send: %q", s)
	n, err := io.WriteString(c.Pty.TerminalInPipe(), s)
	for _, observer := range c.opts.SendObservers {
		observer(s, n, err)
	}
	return n, err
}

// SendLine writes string s to Console's tty with a trailing newline.
func (c *Console) SendLine(s string) (int, error) {
	return c.Send(fmt.Sprintf("%s%s", s, osutils.LineSep))
}

// Log prints to Console's logger.
// Arguments are handled in the manner of fmt.Print.
func (c *Console) Log(v ...interface{}) {
	c.opts.Logger.Print(v...)
}

// Logf prints to Console's logger.
// Arguments are handled in the manner of fmt.Printf.
func (c *Console) Logf(format string, v ...interface{}) {
	c.opts.Logger.Printf(format, v...)
}
