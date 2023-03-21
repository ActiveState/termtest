package termtest_test

import (
	"os/exec"
	"testing"
	"time"

	"github.com/ActiveState/termtest"
	"github.com/stretchr/testify/require"
)

func Test_Basic(t *testing.T) {
	tt, err := termtest.New(exec.Command("bash"), termtest.OptTestErrorHandler(t))
	require.NoError(t, err)
	defer tt.Close()

	tt.ExpectInput()
	tt.SendLine("echo ABC")
	tt.Expect("ABC")
	tt.SendLine("read TEST_OUT")
	tt.SendLine("DEF")
	tt.Expect("DEF")
	tt.SendLine("exit")
	tt.ExpectExitCode(0)
}

func Test_DontMatchInput(t *testing.T) {
	tt, err := termtest.New(exec.Command("bash"), termtest.OptVerboseLogging())
	require.NoError(t, err)
	defer tt.Close()

	tt.SendLine("FOO=bar")
	tt.ExpectInput() // Without this input will be matched
	expectError := tt.Expect("FOO=bar",
		// options:
		termtest.SetTimeout(100*time.Millisecond),
		termtest.SetErrorHandler(termtest.SilenceErrorHandler()), // Prevent errors from bubbling up as panics
	)
	require.ErrorIs(t, expectError, termtest.TimeoutError, "Should have thrown an expect timeout error because FOO=bar was only sent via STDIN, snapshot: %s", tt.Snapshot())

	tt.SendLine("exit")
}
