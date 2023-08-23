package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
)

func Test_Survey_Pty(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to get the current filename")
	}

	c := exec.Command("go", "run", filepath.Join(filepath.Dir(filename), "survey.go"))
	ptyrw, err := pty.StartWithSize(c, &pty.Winsize{Cols: 1000, Rows: 1})
	if err != nil {
		t.Fatalf("pty could not start: %s", err)
	}
	defer func() { _ = ptyrw.Close() }() // Best effort.

	term := vt10x.New(vt10x.WithWriter(ptyrw))
	_ = term

	br := bufio.NewReader(ptyrw)
	wg := &sync.WaitGroup{}
	wg.Add(1)

	buf := &bytes.Buffer{}
	termBuffer := bufio.NewReadWriter(bufio.NewReader(buf), bufio.NewWriter(buf))
	// termBuffer := &bytes.Buffer{}
	ptyReader := io.TeeReader(br, termBuffer)

	// Digest the pty output. We want to read directly from the pty to ensure we only consider new output.
	go func() {
		defer wg.Done()
		for {
			snapshot := make([]byte, 1024)
			n, err := ptyReader.Read(snapshot)
			if n > 0 {
				if _, err := term.Write(snapshot[:n]); err != nil {
					fmt.Printf("Term write error: %s", err)
				}
				fmt.Printf("Received: %s\n", snapshot[:n])
				if strings.Index(string(snapshot[:n]), "Confirm?") != -1 {
					ptyrw.Write([]byte("y\n\n"))
				}
			}
			if err != nil {
				fmt.Printf("read error: %s", err)
				return
			}
		}
	}()

	wg.Wait()
}

func Test_Survey_Pty_Old(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to get the current filename")
	}

	c := exec.Command("go", "run", filepath.Join(filepath.Dir(filename), "survey.go"))
	ptyrw, err := pty.StartWithSize(c, &pty.Winsize{Cols: 1000, Rows: 10})
	if err != nil {
		t.Fatalf("pty could not start: %s", err)
	}
	defer func() { _ = ptyrw.Close() }() // Best effort.

	br := bufio.NewReader(ptyrw)
	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		for {
			snapshot := make([]byte, 1024)
			n, err := br.Read(snapshot)
			if err != nil {
				t.Fatalf("read error: %s", err)
				return
			}
			if n > 0 {
				fmt.Printf("Received: %s\n", snapshot[:n])
				if strings.Index(string(snapshot[:n]), "Confirm?") != -1 {
					ptyrw.Write([]byte("y\n"))
				}
			}
		}
	}()

	wg.Wait()
}
