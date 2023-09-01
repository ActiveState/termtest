package termtest

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
				require.NoError(t, tt.Wait(5*time.Second), "Output: %s", tt.Output())
			}
		})
	}
}

func Test_SendAndOutput(t *testing.T) {
	var cols int = 20
	strColWidth := strings.Repeat("o", cols)
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
			send:     "echo " + strColWidth,
			expect:   strColWidth,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tt := tc.termtest(t)

			tt.SendLine(tc.send)
			tt.SendLine("exit")
			tt.ExpectExitCode(0)

			output := tt.Output()
			require.Contains(t, output, tc.expect, fmt.Sprintf("Expected: %s\nOutput: %s\n", tc.expect, output))
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

func Test_DefaultTimeout(t *testing.T) {
	tt := newTermTest(t, exec.Command("bash", "-c", "sleep .5 && echo MATCH"), true, OptDefaultTimeout(100*time.Millisecond))
	err := tt.Expect("MATCH", OptExpectSilenceErrorHandler())
	require.Error(t, err)
	require.ErrorIs(t, err, TimeoutError)
	time.Sleep(1000 * time.Millisecond)
	tt.Expect("MATCH")
	tt.Wait(time.Second)
}

func Test_PendingOutput_Output_Snapshot(t *testing.T) {
	tt := newTermTest(t, exec.Command("bash", "-c", "echo MATCH1 MATCH2 MATCH3"), true)
	tt.Expect("MATCH1")
	assert.Contains(t, strings.TrimRight(tt.PendingOutput(), "\r\n"), " MATCH2 MATCH3")
	tt.Expect("MATCH2")
	assert.Contains(t, strings.TrimRight(tt.PendingOutput(), "\r\n"), " MATCH3")
	tt.ExpectExitCode(0)
	assert.Contains(t, strings.TrimRight(tt.Output(), "\r\n"), "MATCH1 MATCH2 MATCH3")
	assert.Contains(t, strings.TrimRight(tt.Snapshot(), "\r\n"), "MATCH1 MATCH2 MATCH3")
}

func Test_ColSize(t *testing.T) {
	size := 100
	shell := "bash"
	if runtime.GOOS == "windows" {
		shell = "cmd.exe"
	}
	tt := newTermTest(t, exec.Command(shell), true, OptCols(size))
	v := strings.Repeat("a", size)
	tt.SendLine("echo " + v)
	tt.Expect(v)
	tt.SendLine("exit")
	tt.ExpectExitCode(0)

	// Also test that the terminal snapshot has the right col size
	require.Contains(t, tt.Snapshot(), v)
}

func Test_Multiline_Sanitized(t *testing.T) {
	tt := newTermTest(t, exec.Command("bash"), false, OptOutputSanitizer(func(v []byte) ([]byte, error) {
		return bytes.Replace(v, []byte("\r\n"), []byte("\n"), -1), nil
	}))

	f, err := os.CreateTemp("", "")
	require.NoError(t, err)
	defer os.Remove(f.Name())
	f.WriteString("foo\r\nbar")
	f.Close()

	fpath := f.Name()
	if runtime.GOOS == "windows" {
		fpath = toPosixPath(fpath)
	}

	tt.SendLine("cat " + fpath)
	tt.Expect("foo\nbar")
	tt.SendLine("exit")
	tt.ExpectExitCode(0)
}

func Test_Multiline_Normalized(t *testing.T) {
	tt := newTermTest(t, exec.Command("bash"), false, OptNormalizedLineEnds(true))

	f, err := os.CreateTemp("", "")
	require.NoError(t, err)
	defer os.Remove(f.Name())
	f.WriteString("foo\r\nbar")
	f.Close()

	fpath := f.Name()
	if runtime.GOOS == "windows" {
		fpath = toPosixPath(fpath)
	}

	tt.SendLine("cat " + fpath)
	tt.Expect("foo\nbar")
	tt.SendLine("echo -e \"foo\r\nbar\" | tr -d -c \"\\r\" | wc -c")
	tt.Expect("0") // Should be zero occurrences of \r
	tt.SendLine("exit")
	tt.ExpectExitCode(0)
}
