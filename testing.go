// Copyright 2020 ActiveState Software. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file

package termtest

import (
	"fmt"
	"strings"
	"testing"

	expect "github.com/ActiveState/go-expect"
	"github.com/ActiveState/termtest/internal/osutils/stacktrace"
)

// TestSendObserveFn is an example for a SendObserver function, it reports any error during Send calls to the supplied testing instance
func TestSendObserveFn(t *testing.T) func(string, int, error) {
	return func(msg string, num int, err error) {
		if err == nil {
			return
		}

		t.Fatalf("Could not send data to terminal\nerror: %v", err)
	}
}

// TestExpectObserveFn an example for a ExpectObserver function, it reports any error occurring durint expect calls to the supplied testing instance
func TestExpectObserveFn(t *testing.T) func([]expect.Matcher, string, string, error) {
	return func(matchers []expect.Matcher, raw, pty string, err error) {
		if err == nil {
			return
		}

		var value string
		var sep string
		for _, matcher := range matchers {
			value += fmt.Sprintf("%s%v", sep, matcher.Criteria())
			sep = ", "
		}

		pty = strings.TrimRight(pty, " \n") + "\n"

		t.Fatalf(
			"Could not meet expectation: Expectation: '%s'\nError: %v at\n%s\n---\nTerminal snapshot:\n%s\n---\nParsed output:\n%s\n",
			value, err, stacktrace.Get().String(), pty, raw,
		)
	}
}
