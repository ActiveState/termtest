package termtest

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
	"sync"
	"time"

	"github.com/ActiveState/termtest/internal/osutils"
	"github.com/creack/pty"
)

// TermTest bonds a command with a pseudo-terminal for automation
type TermTest struct {
	cmd            *exec.Cmd
	ptmx           pty.Pty
	outputDigester *outputProducer
	closed         chan struct{}
	listening      bool
	opts           *Opts
}

type Opts struct {
	Logger             *log.Logger
	ExpectErrorHandler func(*TermTest, error) error
	Cols               uint16
	Rows               uint16
}

var TimeoutError = errors.New("timeout")

var neverGonnaHappen = time.Hour * 24 * 365 * 100

type void struct{}

func (v void) Write(p []byte) (n int, err error) { return len(p), nil }

type SetOpt func(o *Opts) error

const DefaultCols = 1000

func New(cmd *exec.Cmd, opts ...SetOpt) (*TermTest, error) {
	optv := &Opts{
		Logger: log.New(void{}, "TermTest: ", log.LstdFlags),
		ExpectErrorHandler: func(_ *TermTest, err error) error {
			panic(err)
		},
		Cols: DefaultCols,
		Rows: 1,
	}
	for _, setOpt := range opts {
		if err := setOpt(optv); err != nil {
			return nil, fmt.Errorf("could not set option: %w", err)
		}
	}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: optv.Cols, Rows: optv.Rows})
	if err != nil {
		return nil, fmt.Errorf("could not start pty: %w", err)
	}

	t := &TermTest{
		cmd:            cmd,
		ptmx:           ptmx,
		outputDigester: newOutputProducer(optv),
		closed:         make(chan struct{}),
		listening:      false,
		opts:           optv,
	}

	// Start listening for output
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		wg.Done()
		err := t.outputDigester.Listen(t.ptmx)
		if err != nil {
			t.opts.Logger.Printf("Error while listening: %s", err)
		}
	}()
	wg.Wait()

	return t, nil
}

// Close cleans up all the resources allocated by the TermTest
func (tt *TermTest) Close() error {
	log.Println("Close called")

	if tt.cmd.ProcessState != nil && !tt.cmd.ProcessState.Exited() {
		return fmt.Errorf("process is still running: %d", tt.cmd.ProcessState.Pid())
	}

	log.Println("Closing pty")
	if err := tt.ptmx.Close(); err != nil {
		return fmt.Errorf("failed to close pty: %w", err)
	}
	log.Println("Closed pty")

	close(tt.closed)

	return nil
}

// Cmd returns the underlying command
func (tt *TermTest) Cmd() *exec.Cmd {
	return tt.cmd
}

// Snapshot returns a string containing a terminal snap-shot as a user would see it in a "real" terminal
func (tt *TermTest) Snapshot() string {
	return string(tt.outputDigester.Snapshot())
}

// Send sends a new line to the terminal, as if a user typed it
func (tt *TermTest) Send(value string) error {
	if isClosed(tt.closed) {
		return tt.opts.ExpectErrorHandler(tt, fmt.Errorf("termtest has already been closed"))
	}
	tt.opts.Logger.Printf("Sending: %s", value)
	_, err := tt.ptmx.Write([]byte(value))
	return err
}

// SendLine sends a new line to the terminal, as if a user typed it, the newline sequence is OS aware
func (tt *TermTest) SendLine(value string) error {
	return tt.Send(fmt.Sprintf("%s%s", value, osutils.LineSep))
}

// SendCtrlC tries to emulate what would happen in an interactive shell, when the user presses Ctrl-C
// Note: On Windows the Ctrl-C event is only reliable caught when the receiving process is
// listening for os.Interrupt signals.
func (tt *TermTest) SendCtrlC() {
	tt.Send(string([]byte{0x03})) // 0x03 is ASCII character for ^C
}
