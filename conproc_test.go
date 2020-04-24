// Copyright 2020 ActiveState Software. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file

package termtest_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	expect "github.com/ActiveState/go-expect"
	"github.com/ActiveState/termtest"
	"github.com/stretchr/testify/suite"
)

type TermTestTestSuite struct {
	suite.Suite
	sessionTester string
	tmpDir        string
}

func (suite *TermTestTestSuite) spawnCustom(retainWorkDir bool, observer expect.ExpectObserver, args ...string) *termtest.ConsoleProcess {
	opts := termtest.Options{
		RetainWorkDir: retainWorkDir,
		ObserveSend:   termtest.TestSendObserveFn(suite.Suite.T()),
		ObserveExpect: observer,
		CmdName:       suite.sessionTester,
		Args:          args,
	}
	cp, err := termtest.New(opts)
	suite.Suite.Require().NoError(err, "create console process")
	return cp
}

func (suite *TermTestTestSuite) spawn(retainWorkDir bool, args ...string) *termtest.ConsoleProcess {
	return suite.spawnCustom(retainWorkDir, termtest.TestExpectObserveFn(suite.Suite.T()), args...)
}

func (suite *TermTestTestSuite) SetupSuite() {
	dir, err := ioutil.TempDir("", "")
	suite.Suite.Require().NoError(err)
	suite.tmpDir = dir

	suite.sessionTester = filepath.Join(dir, "sessionTester")
	fmt.Println(suite.sessionTester)

	cmd := exec.Command("go", "build", "-o", suite.sessionTester, "./cmd/tester")
	err = cmd.Start()
	suite.Suite.Require().NoError(err)
	err = cmd.Wait()
	suite.Suite.Require().NoError(err)
	suite.Suite.Require().Equal(0, cmd.ProcessState.ExitCode())
}

func (suite *TermTestTestSuite) TearDownSuite() {
	err := os.RemoveAll(suite.tmpDir)
	suite.Suite.Require().NoError(err)
}

func (suite *TermTestTestSuite) TestTermTest() {
	buf := make([]byte, 300*80)
	k := 0
	for i := 0; i < 300; i++ {
		copy(buf[k:k+5], []byte(fmt.Sprintf(":%03d:", i)))
		k += 5
		for j := 5; j < 80; j++ {
			buf[k] = byte(fmt.Sprintf("%d", j%10)[0])
			k++
		}
	}
	// terminal size is 80*30 (one newline at end of stream)
	fillbufferOutput := string(buf[len(buf)-80*29:])
	fillRawOutput := string(buf)
	// match at least two consecutive space character
	spaceRe := regexp.MustCompile("  +")
	stexp := make([]string, 0, 20)
	stexpTerm := make([]string, 0, 21)
	stexpTerm = append(stexpTerm, "an expected string")
	for i := 0; i < 20; i++ {
		stexp = append(stexp, fmt.Sprintf("stuttered %d times", i+1))
	}
	stexpTerm = append(stexpTerm, stexp...)
	cases := []struct {
		name           string
		args           []string
		exitCode       int
		terminalOutput string
		withHistory    string
		// Two tests currently fail on Windows (fillBuffer and stuttering). This needs to be fixed.
		skipOnWindows bool
	}{
		{"expect a string", []string{}, 0, "an expected string", "", false},
		{"exit 1", []string{"-exit1"}, 1, "an expected string", "", false},
		{"with filled buffer", []string{"-fill-buffer"}, 0, fillbufferOutput, fillRawOutput, true},
		{"stuttering", []string{"-stutter"}, 0, strings.Join(stexpTerm, " "), strings.Join(stexp, "\n"), true},
	}

	for _, c := range cases {
		suite.Suite.Run(c.name, func() {
			// create a new test-session
			cp := suite.spawn(false, c.args...)
			defer cp.Close()
			_, _ = cp.Expect("an expected string", 10*time.Second)
			_, _ = cp.ExpectExitCode(c.exitCode, 20*time.Second)
			suite.Suite.Equal(c.withHistory, strings.TrimSpace(spaceRe.ReplaceAllString(cp.MatchState().UnwrappedStringToCursorFromMatch(0), "\n")), "raw buffer")
			suite.Suite.Equal(c.terminalOutput, spaceRe.ReplaceAllString(cp.TrimmedSnapshot(), " "), "terminal snapshot")
		})
	}
}

func (suite *TermTestTestSuite) TestExitCode() {
	cases := []struct {
		Name     string
		Args     []string
		ExitCode int
	}{
		{"is-0-expect-1", []string{}, 1},
		{"is-1-expect-0", []string{"-exit1"}, 0},
	}

	for _, c := range cases {
		suite.Suite.Run(c.Name, func() {
			errorFound := false
			cp := suite.spawnCustom(
				false,
				func(matchers []expect.Matcher, ms *expect.MatchState, err error) {
					if err != nil {
						suite.Len(matchers, 1, "one matcher failed")
						suite.Equal(fmt.Sprintf("exit code == %d", c.ExitCode), matchers[0].Criteria())
						errorFound = true
					}
				},
				c.Args...,
			)
			defer cp.Close()
			_, err := cp.ExpectExitCode(c.ExitCode, 10*time.Second)
			suite.Error(err)
			suite.True(errorFound, "expect to observe an error")
		})
	}
}

func (suite *TermTestTestSuite) TestNotExitCode() {
	cases := []struct {
		Name     string
		Args     []string
		ExitCode int
	}{
		{"is-0-expect-not-0", []string{}, 0},
		{"is-1-expect-not-1", []string{"-exit1"}, 1},
	}

	for _, c := range cases {
		suite.Suite.Run(c.Name, func() {
			errorFound := false
			cp := suite.spawnCustom(
				false,
				func(matchers []expect.Matcher, ms *expect.MatchState, err error) {
					if err != nil {
						suite.Len(matchers, 1, "one matcher failed")
						suite.Equal(fmt.Sprintf("exit code != %d", c.ExitCode), matchers[0].Criteria())
						errorFound = true
					}
				},
				c.Args...,
			)
			defer cp.Close()
			_, err := cp.ExpectNotExitCode(c.ExitCode, 10*time.Second)
			suite.Error(err)
			suite.True(errorFound, "expect to observe an error")
		})
	}
}

func (suite *TermTestTestSuite) TestTimeout() {
	var errorFound bool
	cp := suite.spawnCustom(
		false,
		func(matchers []expect.Matcher, ms *expect.MatchState, err error) {
			if err != nil && errors.Is(err, termtest.ErrWaitTimeout) {
				suite.Len(matchers, 1, "one matcher failed")
				suite.Equal("exit code == 0", matchers[0].Criteria())
				errorFound = true
			}
		},
		"-sleep",
	)
	defer cp.Close()
	_, err := cp.ExpectExitCode(0, 100*time.Millisecond)
	suite.Error(err)
	suite.True(errors.Is(err, termtest.ErrWaitTimeout), "expected timeout error, got %v", err)
	suite.True(errorFound, "expect to observe an error")
}

/*
func (suite *TermTestTestSuite) TestInterrupt() {
	// create a new test-session
	cp := suite.spawn(false, "-sleep", "-exit1")
	defer cp.Close()

	_, _ = cp.Expect("an expected string", 10*time.Second)
	cp.SendCtrlC()
	cp.ExpectExitCode(123, 10*time.Second)
}
*/

func TestTermTestTestSuite(t *testing.T) {
	suite.Run(t, new(TermTestTestSuite))
}
