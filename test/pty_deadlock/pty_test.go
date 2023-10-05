package main

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
)

func Test_Pty_Plain_Deadlock(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to get the current filename")
	}
	test_Pty_Deadlock(t, filepath.Join(filepath.Dir(filename), "plain/plain.go"), false)
}

func Test_Pty_Survey_Deadlock(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to get the current filename")
	}
	test_Pty_Deadlock(t, filepath.Join(filepath.Dir(filename), "survey/survey.go"), true)
}

func test_Pty_Deadlock(t *testing.T, script string, emulate bool) {
	total := 100
	c := exec.Command("go", "run", script, strconv.Itoa(total))

	ptyrw, err := pty.StartWithSize(c, &pty.Winsize{Cols: 140, Rows: 10})
	if err != nil {
		t.Fatalf("pty could not start: %s", err)
	}
	defer func() { _ = ptyrw.Close() }()

	var termEmulator vt10x.Terminal
	if emulate {
		termEmulator = vt10x.New(vt10x.WithWriter(ptyrw), vt10x.WithSize(140, 10))
	}

	idx := 0
	pos := 0
	var output []byte
	for {
		snapshot := make([]byte, 1024)
		n, err := ptyrw.Read(snapshot)
		if n > 0 {
			output = append(output, snapshot[:n]...)
			if emulate {
				termEmulator.Write(snapshot[:n])
			}
		}
		fmt.Printf("%d: %s\n", idx, output[pos:])

		answerMsg := fmt.Sprintf("Answer %d: true", idx)
		if found := strings.Index(string(output[pos:]), answerMsg); found != -1 {
			fmt.Println("Answer matched")
			pos += found + len(answerMsg)
			idx = idx + 1
		}

		confirmMsg := fmt.Sprintf("Confirm %d?", idx)
		if found := strings.Index(string(output[pos:]), confirmMsg); found != -1 {
			fmt.Println("Confirm matched")
			pos += found + len(confirmMsg)
			ptyrw.Write([]byte("y\n"))
		}
		if err != nil {
			if err != io.EOF {
				t.Fatalf("read error: %s", err)
			}
			break
		}
	}

	if idx != total {
		t.Fatalf("Did not get the expected number of confirmations, got: %d, want: %d, output:\n%s", idx, total, string(output))
	}
}
