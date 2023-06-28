package termtest

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"sync"
	"testing"
	"time"

	"github.com/ActiveState/pty"
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
	Posix              bool
}

var TimeoutError = errors.New("timeout")

type SetOpt func(o *Opts) error

const DefaultCols = 1000
const DefaultRows = 10

func NewOpts() *Opts {
	return &Opts{
		Logger: log.New(voidWriter{}, "TermTest: ", log.LstdFlags|log.Lshortfile),
		ExpectErrorHandler: func(_ *TermTest, err error) error {
			panic(err)
		},
		Cols:  DefaultCols,
		Rows:  DefaultRows,
		Posix: runtime.GOOS != "windows",
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

func OptCols(cols uint16) SetOpt {
	return func(o *Opts) error {
		o.Cols = cols
		return nil
	}
}

// OptRows sets the number of rows for the pty, increase this if you find your output appears to stop prematurely
// appears to only make a difference on Windows. Linux/Mac will happily function with a single row
func OptRows(rows uint16) SetOpt {
	return func(o *Opts) error {
		o.Rows = rows
		return nil
	}
}

func OptSilenceErrorHandler() SetOpt {
	return OptErrorHandler(SilenceErrorHandler())
}

// OptPosix informs termtest to treat the command as a posix command
// This will affect line endings as well as output sanitization
func OptPosix(v bool) SetOpt {
	return func(o *Opts) error {
		o.Posix = v
		return nil
	}
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
		tt.listenError <- err
	}()
	wg.Wait()

	return nil
}

// Wait will wait for the cmd and pty to close and cleans up all the resources allocated by the TermTest
// For most tests you probably want to use ExpectExit* instead.
// Note that unlike ExpectExit*, this will NOT invoke cmd.Wait().
func (tt *TermTest) Wait(timeout time.Duration) error {
	tt.opts.Logger.Println("wait called")
	defer tt.opts.Logger.Println("wait closed")

	errc := make(chan error, 1)
	go func() {
		errc <- tt.WaitIndefinitely()
	}()

	select {
	case err := <-errc:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("timeout after %s while waiting for command and pty to close: %w", timeout, TimeoutError)
	}
}

func (tt *TermTest) WaitIndefinitely() error {
	tt.opts.Logger.Println("WaitIndefinitely called")
	defer tt.opts.Logger.Println("WaitIndefinitely closed")

	// On windows there is a race condition where ClosePseudoConsole will hang if we call it around the same
	// time as the parent process exits.
	// This is not a clean solution, as there's no guarantee that 100 milliseconds will be sufficient. But in
	// my tests it has been, and I can't afford to keep digging on this.
	if runtime.GOOS == "windows" {
		time.Sleep(time.Millisecond * 100)
	}

	tt.opts.Logger.Println("Closing pty")
	if err := tt.ptmx.Close(); err != nil {
		if errors.Is(err, ERR_ACCESS_DENIED) {
			// Ignore access denied error - means process has already finished
			tt.opts.Logger.Println("Ignoring access denied error")
		} else {
			return fmt.Errorf("failed to close pty: %w", err)
		}
	}
	tt.opts.Logger.Println("Closed pty")

	// wait outputProducer
	// This should trigger listenError from being written to (on a goroutine)
	tt.opts.Logger.Println("Closing outputProducer")
	if err := tt.outputProducer.close(); err != nil {
		return fmt.Errorf("failed to close output digester: %w", err)
	}
	tt.opts.Logger.Println("Closed outputProducer")

	// listenError will be written to when the process exits, and this is the only reasonable place for us to
	// catch listener errors
	return <-tt.listenError
}

// Cmd returns the underlying command
func (tt *TermTest) Cmd() *exec.Cmd {
	return tt.cmd
}

// Snapshot returns a string containing a terminal snapshot as a user would see it in a "real" terminal
func (tt *TermTest) Snapshot() string {
	return string(tt.outputProducer.Snapshot())
}

// Output is similar to snapshot, except that it returns all output produced, rather than the current snapshot of output
func (tt *TermTest) Output() string {
	return string(tt.outputProducer.Output())
}

// Send sends a new line to the terminal, as if a user typed it
func (tt *TermTest) Send(value string) (rerr error) {
	tt.opts.Logger.Printf("Send: %s\n", value)

	tt.opts.Logger.Printf("Sending: %s", value)
	_, err := tt.ptmx.Write([]byte(value))
	return err
}

// SendLine sends a new line to the terminal, as if a user typed it, the newline sequence is OS aware
func (tt *TermTest) SendLine(value string) (rerr error) {
	lineSep := lineSepPosix
	if !tt.opts.Posix {
		lineSep = lineSepWindows
	}
	return tt.Send(fmt.Sprintf("%s%s", value, lineSep))
}

// SendCtrlC tries to emulate what would happen in an interactive shell, when the user presses Ctrl-C
// Note: On Windows the Ctrl-C event is only reliable caught when the receiving process is
// listening for os.Interrupt signals.
func (tt *TermTest) SendCtrlC() {
	tt.opts.Logger.Printf("SendCtrlC\n")
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
