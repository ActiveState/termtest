package termtest

import (
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_Expect(t *testing.T) {
	tt := newTermTest(t, exec.Command("bash", "-c", "echo HELLO"), true)
	tt.Expect("HELLO")
	tt.ExpectExitCode(0)
}

func Test_Expect_Cmd(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping test on non-windows platform")
	}

	tt := newTermTest(t, exec.Command("cmd", "/c", "echo HELLO"), true)
	tt.Expect("HELLO")
	tt.ExpectExitCode(0)
}

func Test_ExpectRe(t *testing.T) {
	tt := newTermTest(t, exec.Command("bash", "-c", "echo HELLO"), true)
	tt.ExpectRe(regexp.MustCompile(`HEL(LO)`))
	tt.ExpectExitCode(0)
}

func Test_ExpectRe_Cmd(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping test on non-windows platform")
	}

	tt := newTermTest(t, exec.Command("cmd", "/c", "echo HELLO"), true)
	tt.ExpectRe(regexp.MustCompile(`HEL(LO)`))
	tt.ExpectExitCode(0)
}

func Test_ExpectCustom(t *testing.T) {
	customErr := fmt.Errorf("custom error")

	type args struct {
		consumer consumer
		opts     []SetExpectOpt
	}
	tests := []struct {
		name    string
		tt      func(t *testing.T) *TermTest
		args    args
		wantErr error
	}{
		{
			"Simple Hello World Match",
			func(t *testing.T) *TermTest {
				return newTermTest(t, exec.Command("bash", "-c", "echo Hello World"), true)
			},
			args{
				func(buffer string) (endPos int, err error) {
					return indexEndPos(buffer, "Hello World"), nil
				},
				[]SetExpectOpt{},
			},
			nil,
		},
		{
			"No match by process end",
			func(t *testing.T) *TermTest {
				return newTermTest(t, exec.Command("bash", "-c", "echo Hello World"), true)
			},
			args{
				func(buffer string) (endPos int, err error) {
					return 0, nil
				},
				[]SetExpectOpt{OptExpectTimeout(time.Second)},
			},
			TimeoutError,
		},
		{
			"Custom error",
			func(t *testing.T) *TermTest {
				return newTermTest(t, exec.Command("bash", "-c", "echo Custom Error"), true)
			},
			args{
				func(buffer string) (endPos int, err error) {
					return 0, customErr
				},
				[]SetExpectOpt{OptExpectTimeout(time.Second)},
			},
			customErr,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tt := tc.tt(t)
			err := tt.ExpectCustom(tc.args.consumer, append(tc.args.opts, OptExpectSilenceErrorHandler())...)
			require.ErrorIs(t, err, tc.wantErr)
			require.NotErrorIs(t, tt.Wait(5*time.Second), TimeoutError)
		})
	}
}

func Test_ExpectCustom_Cmd(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping test on non-windows platform")
	}

	customErr := fmt.Errorf("custom error")

	type args struct {
		consumer consumer
		opts     []SetExpectOpt
	}
	tests := []struct {
		name    string
		tt      func(t *testing.T) *TermTest
		args    args
		wantErr error
	}{
		{
			"Simple Hello World Match",
			func(t *testing.T) *TermTest {
				return newTermTest(t, exec.Command("cmd", "/c", "echo Hello World"), true)
			},
			args{
				func(buffer string) (endPos int, err error) {
					return indexEndPos(buffer, "Hello World"), nil
				},
				[]SetExpectOpt{},
			},
			nil,
		},
		{
			"No match by process end",
			func(t *testing.T) *TermTest {
				return newTermTest(t, exec.Command("cmd", "/c", "echo Hello World"), true)
			},
			args{
				func(buffer string) (endPos int, err error) {
					return 0, nil
				},
				[]SetExpectOpt{OptExpectTimeout(time.Second)},
			},
			TimeoutError,
		},
		{
			"Custom error",
			func(t *testing.T) *TermTest {
				return newTermTest(t, exec.Command("cmd", "/c", "echo Custom Error"), true)
			},
			args{
				func(buffer string) (endPos int, err error) {
					return 0, customErr
				},
				[]SetExpectOpt{OptExpectTimeout(time.Second)},
			},
			customErr,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tt := tc.tt(t)
			err := tt.ExpectCustom(tc.args.consumer, append(tc.args.opts, OptExpectSilenceErrorHandler())...)
			require.ErrorIs(t, err, tc.wantErr)
			require.NotErrorIs(t, tt.Wait(5*time.Second), TimeoutError)
		})
	}
}

func Test_ExpectMatchTwiceSameBuffer(t *testing.T) {
	tt := newTermTest(t, exec.Command("bash"), false)

	tt.Expect("$") // Wait for bash to be ready, so we don't timeout on bash startup

	tt.SendLine("echo ONE TWO THREE")
	tt.Expect("echo ONE TWO THREE", OptExpectTimeout(time.Second)) // Match stdin first

	tt.Expect("ONE", OptExpectTimeout(time.Second))
	err := tt.Expect("ONE", OptExpectTimeout(time.Second), OptExpectSilenceErrorHandler())
	require.ErrorIs(t, err, TimeoutError)

	tt.Expect("TWO", OptExpectTimeout(time.Second))
	err = tt.Expect("TWO", OptExpectTimeout(time.Second), OptExpectSilenceErrorHandler())
	require.ErrorIs(t, err, TimeoutError)

	tt.Expect("THREE", OptExpectTimeout(time.Second))
	err = tt.Expect("THREE", OptExpectTimeout(time.Second), OptExpectSilenceErrorHandler())
	require.ErrorIs(t, err, TimeoutError)

	tt.SendLine("exit")

	tt.ExpectExitCode(0)
}
