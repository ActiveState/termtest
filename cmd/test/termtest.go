package main

import (
	"fmt"
	"os/exec"

	"github.com/creack/pty"
)

type TermTest struct {
	pty pty.Pty
}

func New(cmd *exec.Cmd) (*TermTest, error) {
	tt := &TermTest{}
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 1000, Rows: 1})
	if err != nil {
		return nil, err
	}
	tt.pty = ptmx
	return tt, nil
}

func (t *TermTest) Send(value string) {
	t.SendRaw([]byte(value))
}

func (t *TermTest) SendRaw(value []byte) {
	t.pty.Write(value)
}

func (t *TermTest) Close() error {
	if err := t.pty.Close(); err != nil {
		return fmt.Errorf("failed to close pty: %w", err)
	}
	return nil
}
