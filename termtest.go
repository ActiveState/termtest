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
	"github.com/hinshun/vt10x"
)

// TermTest bonds a command with a pseudo-terminal for automation
type TermTest struct {
	cmd            *exec.Cmd
	term           vt10x.Terminal
	ptmx           pty.Pty
	outputProducer *outputProducer
	listenError    chan error
	opts           *Opts
}

type ErrorHandler func(*TermTest, error) error

type Opts struct {
	Logger             *log.Logger
	ExpectErrorHandler ErrorHandler
	Cols               int
	Rows               int
	Posix              bool
	DefaultTimeout     time.Duration
	OutputSanitizer    cleanerFunc
	NormalizedLineEnds bool
}

var TimeoutError = errors.New("timeout")

var VerboseLogger = log.New(os.Stderr, "TermTest: ", log.LstdFlags|log.Lshortfile)

var VoidLogger = log.New(voidLogger{}, "", 0)

type SetOpt func(o *Opts) error

const DefaultCols = 140
const DefaultRows = 10

func NewOpts() *Opts {
	return &Opts{
		Logger: VoidLogger,
		ExpectErrorHandler: func(_ *TermTest, err error) error {
			panic(err)
		},
		Cols:           DefaultCols,
		Rows:           DefaultRows,
		Posix:          runtime.GOOS != "windows",
		DefaultTimeout: 5 * time.Second,
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
		t.Errorf("Error encountered: %s\nOutput: %s\nStack: %s", unwrapErrorMessage(err), tt.Output(), debug.Stack())
		return err
	}
}

func SilenceErrorHandler() ErrorHandler {
	return func(_ *TermTest, err error) error {
		return err
	}
}

func OptVerboseLogger() SetOpt {
	return OptLogger(VerboseLogger)
}

func OptLogger(logger *log.Logger) SetOpt {
	return func(o *Opts) error {
		o.Logger = logger
		return nil
	}
}

type testLogger struct {
	t *testing.T
}

func (l *testLogger) Write(p []byte) (n int, err error) {
	l.t.Log(string(p))
	return len(p), nil
}

func OptSetTest(t *testing.T) SetOpt {
	return func(o *Opts) error {
		setTest(o, t)
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

func OptCols(cols int) SetOpt {
	return func(o *Opts) error {
		o.Cols = cols
		return nil
	}
}

// OptRows sets the number of rows for the pty, increase this if you find your output appears to stop prematurely
// appears to only make a difference on Windows. Linux/Mac will happily function with a single row
func OptRows(rows int) SetOpt {
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

// OptDefaultTimeout sets the default timeout
func OptDefaultTimeout(duration time.Duration) SetOpt {
	return func(o *Opts) error {
		o.DefaultTimeout = duration
		return nil
	}
}

func OptOutputSanitizer(f cleanerFunc) SetOpt {
	return func(o *Opts) error {
		o.OutputSanitizer = f
		return nil
	}
}

func OptNormalizedLineEnds(v bool) SetOpt {
	return func(o *Opts) error {
		o.NormalizedLineEnds = v
		return nil
	}
}

func (tt *TermTest) SetErrorHandler(handler ErrorHandler) {
	tt.opts.ExpectErrorHandler = handler
}

func (tt *TermTest) SetLogger(logger *log.Logger) {
	tt.opts.Logger = logger
}

func (tt *TermTest) SetTest(t *testing.T) {
	setTest(tt.opts, t)
}

func setTest(o *Opts, t *testing.T) {
	o.Logger = log.New(&testLogger{t}, "TermTest: ", log.LstdFlags|log.Lshortfile)
	o.ExpectErrorHandler = TestErrorHandler(t)
}

func (tt *TermTest) start() (rerr error) {
	expectOpts, err := NewExpectOpts()
	defer tt.ExpectErrorHandler(&rerr, expectOpts)
	if err != nil {
		return fmt.Errorf("could not create expect options: %w", err)
	}

	if tt.ptmx != nil {
		return fmt.Errorf("already started")
	}

	ptmx, err := pty.StartWithSize(tt.cmd, &pty.Winsize{Cols: uint16(tt.opts.Cols), Rows: uint16(tt.opts.Rows)})
	if err != nil {
		return fmt.Errorf("could not start pty: %w", err)
	}
	tt.ptmx = ptmx

	tt.term = vt10x.New(vt10x.WithWriter(ptmx), vt10x.WithSize(tt.opts.Cols, tt.opts.Rows))

	// Start listening for output
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer tt.opts.Logger.Printf("termtest finished listening")
		wg.Done()
		err := tt.outputProducer.Listen(tt.ptmx, tt.term)
		tt.listenError <- err
	}()
	wg.Wait()

	return nil
}

// Wait will wait for the cmd and pty to close and cleans up all the resources allocated by the TermTest
// For most tests you probably want to use ExpectExit* instead.
// Note that unlike ExpectExit*, this will NOT invoke cmd.Wait().
func (tt *TermTest) Wait(timeout time.Duration) (rerr error) {
	tt.opts.Logger.Println("wait called")
	defer tt.opts.Logger.Println("wait closed")

	errc := make(chan error, 1)
	go func() {
		errc <- tt.WaitIndefinitely()
	}()

	select {
	case err := <-errc:
		// WaitIndefinitely already invokes the expect error handler
		return err
	case <-time.After(timeout):
		expectOpts, err := NewExpectOpts()
		defer tt.ExpectErrorHandler(&rerr, expectOpts)
		if err != nil {
			return fmt.Errorf("could not create expect options: %w", err)
		}
		return fmt.Errorf("timeout after %s while waiting for command and pty to close: %w", timeout, TimeoutError)
	}
}

// Cmd returns the underlying command
func (tt *TermTest) Cmd() *exec.Cmd {
	return tt.cmd
}

// Snapshot returns a string containing a terminal snapshot as a user would see it in a "real" terminal
func (tt *TermTest) Snapshot() string {
	return tt.term.String()
}

// PendingOutput returns any output produced that has not yet been matched against
func (tt *TermTest) PendingOutput() string {
	return string(tt.outputProducer.PendingOutput())
}

// Output is similar to snapshot, except that it returns all output produced, rather than the current snapshot of output
func (tt *TermTest) Output() string {
	return string(tt.outputProducer.Output())
}

// Send sends a new line to the terminal, as if a user typed it
func (tt *TermTest) Send(value string) (rerr error) {
	expectOpts, err := NewExpectOpts()
	defer tt.ExpectErrorHandler(&rerr, expectOpts)
	if err != nil {
		return fmt.Errorf("could not create expect options: %w", err)
	}

	// Todo: Drop this sleep and figure out why without this we seem to be running into a race condition.
	// Disabling this sleep will make survey_test.go fail on occasion (rerun it a few times).
	time.Sleep(time.Millisecond)
	tt.opts.Logger.Printf("Send: %s\n", value)
	_, err = tt.ptmx.Write([]byte(value))
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
