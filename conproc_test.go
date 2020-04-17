// Copyright 2020 ActiveState Software. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file

package termtest_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

func (suite *TermTestTestSuite) spawnCustom(retainWorkDir bool, observer termtest.ExpectObserver, args ...string) *termtest.ConsoleProcess {
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

func (suite *TermTestTestSuite) TestE2eSession() {
	// terminal size is 80*30 (one newline at end of stream)
	fillbufferOutput := string(bytes.Repeat([]byte("a"), 80*29))
	// match at least two consecutive space character
	spaceRe := regexp.MustCompile("  +")
	cases := []struct {
		name           string
		args           []string
		exitCode       int
		terminalOutput string
	}{
		{"expect a string", []string{}, 0, "an expected string"},
		{"exit 1", []string{"-exit1"}, 1, "an expected string"},
		{"with filled buffer", []string{"-fill-buffer"}, 0, fillbufferOutput},
		{"stuttering", []string{"-stutter"}, 0, "an expected string stuttered 1 times stuttered 2 times stuttered 3 times stuttered 4 times stuttered 5 times"},
	}

	for _, c := range cases {
		suite.Suite.Run(c.name, func() {
			// create a new test-session
			cp := suite.spawn(false, c.args...)
			defer cp.Close()
			cp.Expect("an expected string", 10*time.Second)
			cp.ExpectExitCode(c.exitCode, 20*time.Second)
			// XXX: On Azure CI pipelines, the terminal output cannot be matched.  Needs investigation and a fix.
			if os.Getenv("CI") != "azure" {
				suite.Suite.Equal(c.terminalOutput, spaceRe.ReplaceAllString(cp.TrimmedSnapshot(), " "))
			}
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
				func(matchers []expect.Matcher, raw, pty string, err error) {
					if err != nil {
						suite.Len(matchers, 1, "one matcher failed")
						suite.Equal(fmt.Sprintf("exit code == %d", c.ExitCode), matchers[0].Criteria())
						errorFound = true
					}
				},
				c.Args...,
			)
			defer cp.Close()
			cp.ExpectExitCode(c.ExitCode, 10*time.Second)
			suite.True(errorFound)
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
				func(matchers []expect.Matcher, raw, pty string, err error) {
					if err != nil {
						suite.Len(matchers, 1, "one matcher failed")
						suite.Equal(fmt.Sprintf("exit code != %d", c.ExitCode), matchers[0].Criteria())
						errorFound = true
					}
				},
				c.Args...,
			)
			defer cp.Close()
			cp.ExpectNotExitCode(c.ExitCode, 10*time.Second)
			suite.True(errorFound)
		})
	}
}

func (suite *TermTestTestSuite) TestE2eSessionInterrupt() {
	if os.Getenv("CI") == "azure" {
		suite.Suite.T().Skip("session interrupt not working on Azure CI ATM")
	}
	// create a new test-session
	cp := suite.spawn(false, "-sleep", "-exit1")
	defer cp.Close()

	cp.Expect("an expected string", 10*time.Second)
	cp.SendCtrlC()
	cp.ExpectExitCode(123, 10*time.Second)
}
func TestTermTestTestSuite(t *testing.T) {
	suite.Run(t, new(TermTestTestSuite))
}
