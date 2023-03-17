package termtest

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os/exec"
	"sync"

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
		outputDigester: newOutputProducer(optv),
		closed:         make(chan struct{}),
		listening:      false,
		opts:           optv,
	}

	if err := t.start(); err != nil {
		return nil, fmt.Errorf("could not start: %w", err)
	}

	return t, nil
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
		wg.Done()
		err := tt.outputDigester.Listen(tt.ptmx)
		if err != nil {
			if !errors.Is(err, fs.ErrClosed) && !errors.Is(err, io.EOF) {
				tt.opts.Logger.Printf("error while listening: %s", err)
				// todo: Find a way to bubble up this error
			}
		}
	}()
	wg.Wait()

	return nil
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

	if err := tt.outputDigester.close(); err != nil {
		return fmt.Errorf("failed to close output digester: %w", err)
	}

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
