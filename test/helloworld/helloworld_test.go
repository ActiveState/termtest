package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ActiveState/termtest"
)

func Test_ExactOutput(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to get the current filename")
	}
	cmd := exec.Command("go", "run", filepath.Join(filepath.Dir(filename), "helloworld.go"))

	tt, err := termtest.New(cmd, termtest.OptTestErrorHandler(t))
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	tt.ExpectExitCode(0)

	output := tt.Output()
	if output != "Hello World" {
		t.Errorf("Output should be 'Hello World' and nothing else, got: %s", output)
	}
}
