package termtest

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		wantErr func(*testing.T, error)
	}{
		{
			"Simple Hello World Match",
			func(t *testing.T) *TermTest {
				return newTermTest(t, exec.Command("bash", "-c", "echo Hello World"), true)
			},
			args{
				func(buffer string) (stopConsuming bool, err error) {
					fmt.Printf("--- buffer: %s (%v)\n", buffer, strings.TrimSpace(buffer) == "Hello World")
					return strings.TrimSpace(buffer) == "Hello World", nil
				},
				[]SetExpectOpt{},
			},
			func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			"No match by process end",
			func(t *testing.T) *TermTest {
				return newTermTest(t, exec.Command("bash", "-c", "echo Hello World"), true)
			},
			args{
				func(buffer string) (stopConsuming bool, err error) {
					return false, nil
				},
				[]SetExpectOpt{SetTimeout(time.Second)},
			},
			func(t *testing.T, err error) {
				assert.ErrorIs(t, err, TimeoutError)
			},
		},
		{
			"Custom error",
			func(t *testing.T) *TermTest {
				return newTermTest(t, exec.Command("bash", "-c", "echo Custom Error"), true)
			},
			args{
				func(buffer string) (stopConsuming bool, err error) {
					fmt.Printf("--- Returning customErr\n")
					return true, customErr
				},
				[]SetExpectOpt{SetTimeout(time.Second)},
			},
			func(t *testing.T, err error) {
				assert.ErrorIs(t, err, customErr)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tt := tc.tt(t)
			err := tt.ExpectCustom(tc.args.consumer, tc.args.opts...)
			tc.wantErr(t, err)
		})
	}
}

func Test_ExpectDontMatchInput(t *testing.T) {
	var expectError error
	tt, err := New(exec.Command("bash"), func(o *Opts) error {
		o.ExpectErrorHandler = func(tt *TermTest, err error) error {
			expectError = err
			return err
		}
		return nil
	})
	require.NoError(t, err)
	defer tt.Close()

	tt.SendLine("FOO=bar")
	tt.ExpectInput() // Without this input will be matched
	tt.Expect("FOO=bar", SetTimeout(100*time.Millisecond))

	require.ErrorIs(t, expectError, TimeoutError, "Should have thrown an expect timeout error because FOO=bar was only sent via STDIN, snapshot: %s", tt.Snapshot())
}
