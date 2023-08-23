package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ActiveState/termtest"
)

// Test_Survey tests whether termtest can handle commands that rely on a full terminal
// The survey package will send cursor instructions to the terminal, if termtest doesn't handle these it will likely
// result in an expect timing out or hanging altogether.
// See commit 39373e6d1dad6c37d2beff134a53bf9ba377022d
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
