package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

func Test_Pty_Output(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to get the current filename")
	}
	c := exec.Command("go", "run", filepath.Join(filepath.Dir(filename), "helloworld.go"))

	ptyrw, err := pty.StartWithSize(c, &pty.Winsize{Cols: 140, Rows: 10})
	if err != nil {
		t.Fatalf("pty could not start: %s", err)
	}
	defer func() { _ = ptyrw.Close() }()

	var output []byte
	for {
		snapshot := make([]byte, 1024)
		n, err := ptyrw.Read(snapshot)
		if n > 0 {
			fmt.Printf("Read: %s", snapshot[:n])
			output = append(output, snapshot[:n]...)
		}
		if err != nil {
			t.Fatalf("read error: %s", err)
			break
		}
		time.Sleep(time.Millisecond * 100)
	}
	outputv := strings.TrimSpace(string(output))
	if outputv != "hello" {
		t.Fatalf("Output is not 'hello', got: %s", outputv)
	}
}
