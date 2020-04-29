// Copyright 2020 ActiveState Software. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file

package xpty_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ActiveState/termtest/xpty"
	"github.com/stretchr/testify/require"
)

func TestXpty(t *testing.T) {
	xp, err := xpty.New(44, 10, false)
	require.NoError(t, err, "creating xpty")

	cmd := exec.Command("go", "run", "./cmd/tester")
	err = xp.StartProcessInTerminal(cmd)
	require.NoError(t, err, "starting test programme")

	go func() {
		defer func() {
			err := xp.Close()
			if err != nil {
				t.Errorf("error closing xpty: %v", err)
			}
		}()

		err := cmd.Wait()
		if err != nil {
			t.Errorf("error waiting for testing programme: %v", err)
		}
	}()

	out := &bytes.Buffer{}

	done := make(chan struct{})
	go func() {
		defer func() { done <- struct{}{} }()
		n, err := xp.WriteTo(out)
		if err != nil && err != io.EOF {
			// On Linux and Windows we can receive a /dev/pts or |1 already closed error
			// But it doesn't always happen! Fun!
			if perr := (*os.PathError)(nil); !errors.As(err, &perr) {
				t.Errorf("got: %T, want: %T", err, perr)
			}
		}
		fmt.Printf("Read %d bytes from terminal\n", n)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		fmt.Println("tester programme did not exit after ten seconds.")
	}

	outs := strings.ReplaceAll(out.String(), "\033", "<esc>")
	fmt.Printf("Raw terminal output:\n%s\n", outs)

	if !strings.Contains(outs, "cursor is at row 5 and col 10") {
		log.Fatal("wrong cursor position: expected 'row 5 and col 10'")
	}

	terminalOut := xp.State.String()
	terminalLines := len(strings.Split(terminalOut, "\n"))
	if terminalLines != 11 {
		t.Errorf("expected 11 rows in terminal, got %d", terminalLines)
	}
	fmt.Printf("Formatted terminal output:\n%s\n", xp.State.String())
	xp.WaitTillDrained()
}
