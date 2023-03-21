package termtest

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"runtime/debug"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

// TermTest bonds a command with a pseudo-terminal for automation
type TermTest struct {
	cmd            *exec.Cmd
	ptmx           pty.Pty
	outputProducer *outputProducer
	listenError    chan error
	opts           *Opts
}

type ErrorHandler func(*TermTest, error) error

type Opts struct {
	Logger             *log.Logger
	ExpectErrorHandler ErrorHandler
	Cols               uint16
	Rows               uint16
}

var TimeoutError = errors.New("timeout")

type SetOpt func(o *Opts) error

const DefaultCols = 1000

func NewOpts() *Opts {
	return &Opts{
		Logger: log.New(voidWriter{}, "TermTest: ", log.LstdFlags|log.Lshortfile),
		ExpectErrorHandler: func(_ *TermTest, err error) error {
			panic(err)
		},
		Cols: DefaultCols,
		Rows: 1,
	}
}

func New(cmd *exec.Cmd, opts ...SetOpt) (*TermTest, error) {
	optv := NewOpts()
	for _, setOpt := range opts {
		if err := setOpt(optv); err != nil {
			return nil, fmt.Errorf("could not set option: %w", err)
		}
	}

	t := &TermTest{
		cmd:            cmd,
		outputProducer: newOutputProducer(optv),
		listenError:    make(chan error, 1),
		opts:           optv,
	}

	if err := t.start(); err != nil {
		return nil, fmt.Errorf("could not start: %w", err)
	}

	return t, nil
}

func TestErrorHandler(t *testing.T) ErrorHandler {
	return func(tt *TermTest, err error) error {
		t.Errorf("Error encountered: %s\nSnapshot: %s\nStack: %s", unwrapErrorMessage(err), tt.Snapshot(), debug.Stack())
		return err
	}
}

func SilenceErrorHandler() ErrorHandler {
	return func(_ *TermTest, err error) error {
		return err
	}
}

func OptVerboseLogging() SetOpt {
	return func(o *Opts) error {
		o.Logger = log.New(os.Stderr, "TermTest: ", log.LstdFlags|log.Lshortfile)
		return nil
	}
}

func OptErrorHandler(handler ErrorHandler) SetOpt {
	return func(o *Opts) error {
		o.ExpectErrorHandler = handler
		return nil
	}
}

func OptTestErrorHandler(t *testing.T) SetOpt {
	return OptErrorHandler(TestErrorHandler(t))
}

func OptSilenceErrorHandler() SetOpt {
	return OptErrorHandler(SilenceErrorHandler())
}

func (tt *TermTest) start() error {
	if tt.ptmx != nil {
		return fmt.Errorf("already started")
	}

	ptmx, err := pty.StartWithSize(tt.cmd, &pty.Winsize{Cols: tt.opts.Cols, Rows: tt.opts.Rows})
	if err != nil {
		return fmt.Errorf("could not start pty: %w", err)
	}

	tt.ptmx = ptmx

	// Start listening for output
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer tt.opts.Logger.Printf("termtest finished listening")
		wg.Done()
		err := tt.outputProducer.Listen(tt.ptmx)
		if err != nil {
			if err == nil || errors.Is(err, fs.ErrClosed) || errors.Is(err, io.EOF) {
				tt.listenError <- nil
				return
			}
		}
		tt.listenError <- err
	}()
	wg.Wait()

	return nil
}

// Close cleans up all the resources allocated by the TermTest
func (tt *TermTest) Close() (rerr error) {
	defer tt.errorHandler(&rerr)

	tt.opts.Logger.Println("Close called")
	defer tt.opts.Logger.Println("Closed")

	// Wait for command exit
	cmdError := make(chan error, 1)
	go func() {
		cmdError <- tt.cmd.Wait()
	}()
	select {
	case err := <-cmdError:
		if err != nil {
			// Ignore ECHILD (no child process) error - means process has already finished
			if !errors.Is(err, syscall.ECHILD) {
				return fmt.Errorf("failed to wait for command: %w", err)
			}
		}
	case <-time.After(time.Second):
		return fmt.Errorf("timeout waiting for command to exit")
	}

	// Close pty
	tt.opts.Logger.Println("Closing pty")
	if err := tt.ptmx.Close(); err != nil {
		return fmt.Errorf("failed to close pty: %w", err)
	}
	tt.opts.Logger.Println("Closed pty")

	// Close outputProducer
	// This should trigger listenError from being written to (on a goroutine)
	tt.opts.Logger.Println("Closing outputProducer")
	if err := tt.outputProducer.close(); err != nil {
		return fmt.Errorf("failed to close output digester: %w", err)
	}
	tt.opts.Logger.Println("Closed outputProducer")

	return <-tt.listenError
}

// Cmd returns the underlying command
func (tt *TermTest) Cmd() *exec.Cmd {
	return tt.cmd
}

// Snapshot returns a string containing a terminal snap-shot as a user would see it in a "real" terminal
func (tt *TermTest) Snapshot() string {
	return string(tt.outputProducer.Snapshot())
}

// Send sends a new line to the terminal, as if a user typed it
func (tt *TermTest) Send(value string) (rerr error) {
	tt.opts.Logger.Printf("Sending: %s", value)
	_, err := tt.ptmx.Write([]byte(value))
	return err
}

// SendLine sends a new line to the terminal, as if a user typed it, the newline sequence is OS aware
func (tt *TermTest) SendLine(value string) (rerr error) {
	return tt.Send(fmt.Sprintf("%s%s", value, lineSep))
}

// SendCtrlC tries to emulate what would happen in an interactive shell, when the user presses Ctrl-C
// Note: On Windows the Ctrl-C event is only reliable caught when the receiving process is
// listening for os.Interrupt signals.
func (tt *TermTest) SendCtrlC() {
	tt.Send(string([]byte{0x03})) // 0x03 is ASCII character for ^C
}

func (tt *TermTest) errorHandler(rerr *error) {
	err := *rerr
	if err == nil {
		return
	}

	*rerr = tt.opts.ExpectErrorHandler(tt, err)
	return
}
