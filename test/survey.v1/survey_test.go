package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ActiveState/termtest"
)

func Test_Survey(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to get the current filename")
	}
	cmd := exec.Command("go", "run", filepath.Join(filepath.Dir(filename), "survey.go"))
	tt, err := termtest.New(cmd, termtest.OptTestErrorHandler(t))
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	tt.Expect("Confirm?")
	tt.SendLine("y")
	tt.ExpectExitCode(0)
}

func Test_Survey_Bash(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to get the current filename")
	}
	cmd := exec.Command("bash", "-i")
	tt, err := termtest.New(cmd, termtest.OptTestErrorHandler(t))
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	tt.ExpectInput()
	tt.SendLine(fmt.Sprintf("go run %s", filepath.Join(filepath.Dir(filename), "survey.go")))
	tt.Expect("Confirm?")
	tt.SendLine("y")
	tt.SendLine("exit")
	tt.ExpectExitCode(0)
}
