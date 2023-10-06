package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/ActiveState/termtest"
)

func Test_Termtest_Plain_Deadlock(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to get the current filename")
	}
	test_Termtest_Deadlock(t, filepath.Join(filepath.Dir(filename), "plain/plain.go"))
}

func Test_Termtest_Survey_Deadlock(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to get the current filename")
	}
	test_Termtest_Deadlock(t, filepath.Join(filepath.Dir(filename), "survey/survey.go"))
}

func test_Termtest_Deadlock(t *testing.T, script string) {
	total := 100
	cmd := exec.Command("go", "run", script, strconv.Itoa(total))
	tt, err := termtest.New(cmd, termtest.OptTestErrorHandler(t))
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// We run this on a loop in order to surface potential race conditions between expect, send and the terminal emulator
	// See commit ce7a90a87918c7a5ae7a127fe7391abaa59b1ea7
	for x := 0; x < total; x++ {
		t.Run(fmt.Sprintf("Test %d", x), func(t *testing.T) {
			tt.SetErrorHandler(termtest.TestErrorHandler(t))
			tt.Expect(fmt.Sprintf("Confirm %d?", x))
			tt.SendLine("n")
			tt.Expect(fmt.Sprintf("Answer %d: false", x))
		})
	}
	tt.ExpectExitCode(0)
}
