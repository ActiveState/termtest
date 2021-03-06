// Copyright 2018 Netflix, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package expect

import (
	"bytes"
	"errors"
	"io"
	"regexp"
	"testing"

	"github.com/ActiveState/vt10x"
	"github.com/stretchr/testify/require"
)

func TestExpectOptString(t *testing.T) {
	tests := []struct {
		title    string
		opt      ExpectOpt
		data     string
		expected bool
	}{
		{
			"No args",
			String(),
			"Hello world",
			false,
		},
		{
			"Single arg",
			String("world"),
			"Hello world",
			true,
		},
		{
			"Multiple arg",
			String("other", "world"),
			"Hello world",
			true,
		},
		{
			"No matches",
			String("hello"),
			"Hello world",
			false,
		},
		{
			"Long wrapped text",
			LongString("hello \r\nworld\r\nnewline"),
			"hello    world     newline",
			true,
		},
		{
			"Long wrapped text mismatch",
			LongString("Hello \r\nworld\r\nnewline"),
			"hello    world     newline",
			false,
		},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			var options ExpectOpts
			err := test.opt(&options)
			require.Nil(t, err)

			ms := mockMatchState(t, test.data)
			matcher := options.Match(ms)
			if test.expected {
				require.NotNil(t, matcher)
			} else {
				require.Nil(t, matcher)
			}
		})
	}
}

func TestExpectOptRegexp(t *testing.T) {
	tests := []struct {
		title    string
		opt      ExpectOpt
		data     string
		expected bool
	}{
		{
			"No args",
			Regexp(),
			"Hello world",
			false,
		},
		{
			"Single arg",
			Regexp(regexp.MustCompile(`^Hello`)),
			"Hello world",
			true,
		},
		{
			"Multiple arg",
			Regexp(regexp.MustCompile(`^Hello$`), regexp.MustCompile(`world$`)),
			"Hello world",
			true,
		},
		{
			"No matches",
			Regexp(regexp.MustCompile(`^Hello$`)),
			"Hello world",
			false,
		},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			var options ExpectOpts
			err := test.opt(&options)
			require.Nil(t, err)

			ms := mockMatchState(t, test.data)
			matcher := options.Match(ms)
			if test.expected {
				require.NotNil(t, matcher)
			} else {
				require.Nil(t, matcher)
			}
		})
	}
}

func TestExpectOptRegexpPattern(t *testing.T) {
	tests := []struct {
		title    string
		opt      ExpectOpt
		data     string
		expected bool
	}{
		{
			"No args",
			RegexpPattern(),
			"Hello world",
			false,
		},
		{
			"Single arg",
			RegexpPattern(`^Hello`),
			"Hello world",
			true,
		},
		{
			"Multiple arg",
			RegexpPattern(`^Hello$`, `world$`),
			"Hello world",
			true,
		},
		{
			"No matches",
			RegexpPattern(`^Hello$`),
			"Hello world",
			false,
		},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			var options ExpectOpts
			err := test.opt(&options)
			require.Nil(t, err)

			ms := mockMatchState(t, test.data)
			matcher := options.Match(ms)
			if test.expected {
				require.NotNil(t, matcher)
			} else {
				require.Nil(t, matcher)
			}
		})
	}
}

func TestExpectOptError(t *testing.T) {
	tests := []struct {
		title    string
		opt      ExpectOpt
		data     error
		expected bool
	}{
		{
			"No args",
			Error(),
			io.EOF,
			false,
		},
		{
			"Single arg",
			Error(io.EOF),
			io.EOF,
			true,
		},
		{
			"Multiple arg",
			Error(io.ErrShortWrite, io.EOF),
			io.EOF,
			true,
		},
		{
			"No matches",
			Error(io.ErrShortWrite),
			io.EOF,
			false,
		},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			var options ExpectOpts
			err := test.opt(&options)
			require.Nil(t, err)

			matcher := options.Match(test.data)
			if test.expected {
				require.NotNil(t, matcher)
			} else {
				require.Nil(t, matcher)
			}
		})
	}
}

func mockMatchState(t *testing.T, data string) *MatchState {
	buf := new(bytes.Buffer)
	_, err := buf.WriteString(data)
	require.NoError(t, err)

	var st vt10x.State
	st.RecordHistory = true
	st.WriteString(data, 10, 80)

	return &MatchState{
		TermState: &st,
		Buf:       buf,
	}
}

func TestExpectOptThen(t *testing.T) {
	var (
		errFirst  = errors.New("first")
		errSecond = errors.New("second")
	)

	tests := []struct {
		title    string
		opt      ExpectOpt
		data     string
		match    bool
		expected error
	}{
		{
			"Noop",
			String("world").Then(func(ms *MatchState) error {
				return nil
			}),
			"Hello world",
			true,
			nil,
		},
		{
			"Short circuit",
			String("world").Then(func(ms *MatchState) error {
				return errFirst
			}).Then(func(ms *MatchState) error {
				return errSecond
			}),
			"Hello world",
			true,
			errFirst,
		},
		{
			"Chain",
			String("world").Then(func(ms *MatchState) error {
				return nil
			}).Then(func(ms *MatchState) error {
				return errSecond
			}),
			"Hello world",
			true,
			errSecond,
		},
		{
			"No matches",
			String("World").Then(func(ms *MatchState) error {
				return errFirst
			}),
			"Hello world",
			false,
			nil,
		},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			var options ExpectOpts
			err := test.opt(&options)
			require.Nil(t, err)

			ms := mockMatchState(t, test.data)

			matcher := options.Match(ms)
			if test.match {
				require.NotNil(t, matcher)

				cb, ok := matcher.(CallbackMatcher)
				if ok {
					require.True(t, ok)

					err = cb.Callback(nil)
					require.Equal(t, test.expected, err)
				}
			} else {
				require.Nil(t, matcher)
			}
		})
	}
}

func TestExpectOptAll(t *testing.T) {
	tests := []struct {
		title    string
		opt      ExpectOpt
		data     string
		expected bool
	}{
		{
			"No opts",
			All(),
			"Hello world",
			true,
		},
		{
			"Single string match",
			All(String("world")),
			"Hello world",
			true,
		},
		{
			"Single string no match",
			All(String("Hello")),
			"No match",
			false,
		},
		{
			"Ordered strings match",
			All(String("Hello world"), String("world")),
			"Hello world",
			true,
		},
		{
			"Ordered strings not all match",
			All(String("Hello"), String("world")),
			"Hello",
			false,
		},
		{
			"Unordered strings",
			All(String("world"), String("Hello world")),
			"Hello world",
			true,
		},
		{
			"Unordered strings not all match",
			All(String("world"), String("Hello")),
			"Hello",
			false,
		},
		{
			"Repeated strings match",
			All(String("world"), String("world")),
			"Hello world",
			true,
		},
		{
			"Mixed opts match",
			All(String("woxld"), RegexpPattern(`wo[a-z]{1}ld`)),
			"Hello woxld",
			true,
		},
		{
			"Mixed opts no match",
			All(String("wo4ld"), RegexpPattern(`wo[a-z]{1}ld`)),
			"Hello wo4ld",
			false,
		},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			var options ExpectOpts
			err := test.opt(&options)
			require.Nil(t, err)

			ms := mockMatchState(t, test.data)
			matcher := options.Match(ms)
			if test.expected {
				require.NotNil(t, matcher)
			} else {
				require.Nil(t, matcher)
			}
		})
	}
}
