package termtest

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func newTestOpts(o *Opts) *Opts {
	if o == nil {
		o = &Opts{}
	}
	o.Logger = log.New(os.Stderr, "TermTest: ", log.LstdFlags|log.Lmicroseconds)
	o.ExpectErrorHandler = func(t *TermTest, err error) error {
		return fmt.Errorf("Error encountered: %w\nSnapshot: %s", err, t.Snapshot())
		return err
	}
	return o
}

func newTermTest(t *testing.T, cmd *exec.Cmd) *TermTest {
	tt, err := New(cmd, func(o *Opts) error {
		o = newTestOpts(o)
		return nil
	})
	require.NoError(t, err)
	return tt
}

func TestTermTest_Close(t *testing.T) {
	defer goleak.VerifyNone(t)

	wgExpectRunning := &sync.WaitGroup{}
	wgExpectRunning.Add(1)

	tests := []struct {
		name     string
		termtest func(t *testing.T) *TermTest
		wg       *sync.WaitGroup
		wantErr  bool
	}{
		{
			"Simple",
			func(t *testing.T) *TermTest { return newTermTest(t, exec.Command("bash", "--version")) },
			nil,
			false,
		},
		{
			"Expect Running",
			func(t *testing.T) *TermTest {
				tt := newTermTest(t, exec.Command("bash", "--version"))
				go func() {
					defer wgExpectRunning.Done()
					err := tt.Expect("Too late")
					require.ErrorIs(t, err, StopPrematureError)
				}()
				return tt
			},
			wgExpectRunning,
			false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tt := tc.termtest(t)
			if err := tt.Close(); (err != nil) != tc.wantErr {
				t.Errorf("Close() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wg != nil {
				tc.wg.Wait()
			}
		})
	}
}

func TestTermTest_ExpectExitCode(t *testing.T) {
	tests := []struct {
		name      string
		termtest  func(t *testing.T) *TermTest
		send      string
		testLeak  bool
		expectErr error
		expect    int
	}{
		{
			"Simple exit 0",
			func(t *testing.T) *TermTest { return newTermTest(t, exec.Command("bash")) },
			"exit 0",
			true,
			nil,
			0,
		},
		{
			"Simple exit 100",
			func(t *testing.T) *TermTest { return newTermTest(t, exec.Command("bash")) },
			"exit 100",
			true,
			nil,
			100,
		},
		{
			"Timeout",
			func(t *testing.T) *TermTest { return newTermTest(t, exec.Command("bash")) },
			"sleep 10 && exit 0",
			false, // This is due to cmd.Process.Wait() in waitForCmdExit not being interruptable
			TimeoutError,
			0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.testLeak {
				defer goleak.VerifyNone(t)
			}

			tt := tc.termtest(t)
			defer tt.Close()

			require.NoError(t, tt.SendLine(tc.send))
			err := tt.ExpectExitCode(tc.expect, time.Second)
			if !errors.Is(err, tc.expectErr) {
				t.Errorf("ExpectExitCode() error = %v, expectErr %v", err, tc.expectErr)
			}
		})
	}
}

func TestTermTest_SendAndSnapshot(t *testing.T) {
	defer goleak.VerifyNone(t)

	randStr1 := randString(DefaultCols + 1)
	tests := []struct {
		name     string
		termtest func(t *testing.T) *TermTest
		send     string
		expect   string
	}{
		{
			name:     "Hello",
			termtest: func(t *testing.T) *TermTest { return newTermTest(t, exec.Command("bash")) },
			send:     "echo hello",
			expect:   "hello",
		},
		{
			name:     "Long String",
			termtest: func(t *testing.T) *TermTest { return newTermTest(t, exec.Command("bash")) },
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

func randString(n int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
