package termtest

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func newTestOpts(o *Opts, t *testing.T) *Opts {
	if o == nil {
		o = &Opts{}
	}
	o.Logger = log.New(os.Stderr, filepath.Base(t.Name())+": ", log.Ltime|log.Lmicroseconds|log.Lshortfile)
	o.ExpectErrorHandler = func(t *TermTest, err error) error {
		return fmt.Errorf("Error encountered: %w\nSnapshot: %s", err, t.Snapshot())
		return err
	}
	return o
}

func newTermTest(t *testing.T, cmd *exec.Cmd, logging bool) *TermTest {
	tt, err := New(cmd, func(o *Opts) error {
		o = newTestOpts(o, t)
		if !logging {
			o.Logger = log.New(voidWriter{}, "TermTest: ", log.LstdFlags)
		}
		return nil
	})
	require.NoError(t, err)
	return tt
}

func Test_Close(t *testing.T) {
	tests := []struct {
		name     string
		termtest func(t *testing.T, wg *sync.WaitGroup) *TermTest
		wantErr  bool
	}{
		{
			"Simple",
			func(t *testing.T, wg *sync.WaitGroup) *TermTest {
				defer wg.Done()
				return newTermTest(t, exec.Command("bash", "--version"), true)
			},
			false,
		},
		{
			"Late Expect",
			func(t *testing.T, wg *sync.WaitGroup) *TermTest {
				tt := newTermTest(t, exec.Command("bash", "--version"), true)
				go func() {
					defer wg.Done()
					time.Sleep(time.Second) // Ensure that Close is called before we run the Expect
					err := tt.Expect("Too late", SetTimeout(time.Millisecond))
					require.ErrorIs(t, err, TimeoutError)
				}()
				return tt
			},
			false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wg := &sync.WaitGroup{}
			wg.Add(1)
			tt := tc.termtest(t, wg)
			if err := tt.Close(); (err != nil) != tc.wantErr {
				t.Errorf("Close() error = %v, wantErr %v", err, tc.wantErr)
			}
			wg.Wait()
		})
	}
}

func Test_ExpectExitCode(t *testing.T) {
	tests := []struct {
		name      string
		termtest  func(t *testing.T) *TermTest
		send      string
		expectErr error
		expect    int
	}{
		{
			"Simple exit 0",
			func(t *testing.T) *TermTest { return newTermTest(t, exec.Command("bash"), true) },
			"exit 0",
			nil,
			0,
		},
		{
			"Simple exit 100",
			func(t *testing.T) *TermTest { return newTermTest(t, exec.Command("bash"), true) },
			"exit 100",
			nil,
			100,
		},
		{
			"Timeout",
			func(t *testing.T) *TermTest { return newTermTest(t, exec.Command("bash"), true) },
			"sleep 10 && exit 0",
			TimeoutError,
			0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tt := tc.termtest(t)
			defer tt.Close()

			require.NoError(t, tt.SendLine(tc.send))
			err := tt.ExpectExitCode(tc.expect, SetTimeout(time.Second))
			if !errors.Is(err, tc.expectErr) {
				t.Errorf("ExpectExitCode() error = %v, expectErr %v", err, tc.expectErr)
			}
		})
	}
}

func Test_SendAndSnapshot(t *testing.T) {
	// Todo: Figure out why we are leaking goroutines here (ONLY when running the full test suite, not when running individual test)
	// defer goleak.VerifyNone(t)

	randStr1 := randString(DefaultCols + 1)
	tests := []struct {
		name     string
		termtest func(t *testing.T) *TermTest
		send     string
		expect   string
	}{
		{
			name:     "Hello",
			termtest: func(t *testing.T) *TermTest { return newTermTest(t, exec.Command("bash"), true) },
			send:     "echo hello",
			expect:   "hello",
		},
		{
			name:     "Long String",
			termtest: func(t *testing.T) *TermTest { return newTermTest(t, exec.Command("bash"), true) },
			send:     "echo " + randStr1,
			expect:   randStr1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tt := tc.termtest(t)
			defer tt.Close()

			tt.SendLine(tc.send)
			tt.SendLine("exit")
			tt.ExpectExit()
			snapshot := tt.Snapshot()
			require.Contains(t, snapshot, tc.expect)
		})
	}
}

func Test_Timeout(t *testing.T) {
	tt, err := New(exec.Command("bash"), OptVerboseLogging())
	require.NoError(t, err)
	defer tt.Close()

	start := time.Now()
	expectError := tt.Expect("nevergonnamatch",
		// options:
		SetTimeout(100*time.Millisecond),
		SetErrorHandler(SilenceErrorHandler()), // Prevent errors from bubbling up as panics
	)
	require.ErrorIs(t, expectError, TimeoutError)

	// Timing tests are always error prone, so we give it a little wiggle room
	if time.Since(start) > (200 * time.Millisecond) {
		t.Errorf("Expect() took too long to timeout, took %s, but expected it to takes less than 200ms", time.Since(start))
	}

	tt.SendLine("exit")
}

func randString(n int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
