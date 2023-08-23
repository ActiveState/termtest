package main

import (
	"fmt"
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

	// We run this on a loop in order to surface potential race conditions between expect, send and the terminal emulator
	// See commit 90f0511212539829317b2652d57785c7a3cc71f8
	for x := 0; x < 10; x++ {
		tt.Expect(fmt.Sprintf("Confirm %d?", x))
		tt.SendLine("y")
		tt.Expect(fmt.Sprintf("Answer %d: true", x))
	}
	tt.ExpectExitCode(0)
}
