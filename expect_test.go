package termtest

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTermTest_ExpectCustom(t *testing.T) {
	customErr := fmt.Errorf("custom error")

	type args struct {
		consumer consumer
		timeout  time.Duration
		opts     []SetConsOpt
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
				5 * time.Second,
				[]SetConsOpt{},
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
				5 * time.Second,
				[]SetConsOpt{},
			},
			func(t *testing.T, err error) {
				want := &ExpectNotMetDueToStopError{}
				assert.ErrorAs(t, err, &want)
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
				time.Second,
				[]SetConsOpt{},
			},
			func(t *testing.T, err error) {
				assert.ErrorIs(t, err, customErr)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tt := tc.tt(t)
			err := tt.ExpectCustom(tc.args.consumer, tc.args.timeout, tc.args.opts...)
			tc.wantErr(t, err)
		})
	}
}
