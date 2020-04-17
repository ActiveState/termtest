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

	"github.com/ActiveState/termtest"
	"github.com/stretchr/testify/suite"
)

type TermTestTestSuite struct {
	suite.Suite
	sessionTester string
	tmpDir        string
}

func (suite *TermTestTestSuite) spawn(retainWorkDir bool, args ...string) *termtest.ConsoleProcess {
	opts := &termtest.Options{
		RetainWorkDir: retainWorkDir,
		ObserveSend:   termtest.TestSendObserveFn(suite.Suite.T()),
		ObserveExpect: termtest.TestExpectObserveFn(suite.Suite.T()),
		CmdName:       suite.sessionTester,
		Args:          args,
	}
	err := opts.Normalize()
	suite.Suite.Require().NoError(err, "normalize options")

	cp, err := termtest.New(*opts)
	suite.Suite.Require().NoError(err, "create console process")
	return cp
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
