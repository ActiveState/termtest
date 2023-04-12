package termtest

import (
	"fmt"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_Wait(t *testing.T) {
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
					time.Sleep(time.Second) // Ensure that wait is called before we run the Expect
					err := tt.Expect("Too late", OptExpectTimeout(time.Millisecond), OptExpectSilenceErrorHandler())
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
			if err := tt.Wait(time.Second * 5); (err != nil) != tc.wantErr {
				t.Errorf("wait() error = %v, wantErr %v", err, tc.wantErr)
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
		exitAfter bool
		expectErr error
		expect    int
	}{
		{
			"Simple exit 0",
			func(t *testing.T) *TermTest { return newTermTest(t, exec.Command("bash"), true) },
			"exit 0",
			false,
			nil,
			0,
		},
		{
			"Simple exit 100",
			func(t *testing.T) *TermTest { return newTermTest(t, exec.Command("bash"), true) },
			"exit 100",
			false,
			nil,
			100,
		},
		{
			"Timeout",
			func(t *testing.T) *TermTest { return newTermTest(t, exec.Command("bash"), true) },
			"sleep 1.1 && exit 0",
			true,
			TimeoutError,
			0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tt := tc.termtest(t)

			require.NoError(t, tt.SendLine(tc.send))
			err := tt.ExpectExitCode(tc.expect, OptExpectTimeout(time.Second), OptExpectSilenceErrorHandler())
			require.ErrorIs(t, err, tc.expectErr)

			// Without this goleak will complain about a goroutine leak because the command will still be running
			if tc.exitAfter {
				require.NoError(t, tt.Wait(5*time.Second), "Snapshot: %s", tt.Snapshot())
			}
		})
	}
}

func Test_SendAndSnapshot(t *testing.T) {
	var cols uint16 = 20
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
			termtest: func(t *testing.T) *TermTest { return newTermTest(t, exec.Command("bash"), true, OptCols(cols)) },
			send:     "echo hellooooooooooooooooooooooo",
			expect:   "hellooooooooooooooooooooooo",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tt := tc.termtest(t)

			tt.ExpectInput()

			tt.SendLine(tc.send)
			tt.SendLine("exit")

			// There is a race condition between the process exiting (ie. the ExpectExitCode above) and the snapshot
			// reading goroutine finishing.
			// https://activestatef.atlassian.net/browse/DX-1739
			time.Sleep(time.Second)
			
			tt.ExpectExitCode(0)

			snapshot := tt.Snapshot()
			require.Contains(t, snapshot, tc.expect, fmt.Sprintf("Expected: %s\nSnapshot: %s\n", tc.expect, snapshot))
		})
	}
}

func Test_Timeout(t *testing.T) {
	tt := newTermTest(t, exec.Command("bash"), true)

	start := time.Now()
	expectError := tt.Expect("nevergonnamatch",
		// options:
		OptExpectTimeout(100*time.Millisecond),
		OptExpectErrorHandler(SilenceErrorHandler()), // Prevent errors from bubbling up as panics
	)
	require.ErrorIs(t, expectError, TimeoutError)

	// Timing tests are always error prone, so we give it a little wiggle room
	if time.Since(start) > (200 * time.Millisecond) {
		t.Errorf("Expect() took too long to timeout, took %s, but expected it to takes less than 200ms", time.Since(start))
	}

	tt.SendLine("exit")
	tt.ExpectExitCode(0)
}
