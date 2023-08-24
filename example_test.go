package termtest_test

import (
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ActiveState/termtest"
)

func Test_Basic(t *testing.T) {
	tt, err := termtest.New(exec.Command("bash"), termtest.OptTestErrorHandler(t))
	require.NoError(t, err)

	tt.SendLine("echo ABC")
	tt.Expect("ABC")
	tt.SendLine("echo DEF")
	tt.Expect("DEF")
	tt.SendLine("exit")
	tt.ExpectExitCode(0)
}

func Test_DontMatchInput(t *testing.T) {
	tt, err := termtest.New(exec.Command("bash"), termtest.OptVerboseLogging())
	require.NoError(t, err)

	tt.SendLine("FOO=bar")
	tt.Expect("FOO=bar") // This matches the input, not the output
	expectError := tt.Expect("FOO=bar",
		// options:
		termtest.OptExpectTimeout(100*time.Millisecond),
		termtest.OptExpectErrorHandler(termtest.SilenceErrorHandler()), // Prevent errors from bubbling up as panics
	)
	require.ErrorIs(t, expectError, termtest.TimeoutError, "Should have thrown an expect timeout error because FOO=bar was only sent via STDIN, snapshot: %s", tt.Snapshot())

	tt.SendLine("exit")
	tt.ExpectExitCode(0)
}
