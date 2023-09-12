package termtest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_cleanPtySequences(t *testing.T) {
	tests := []struct {
		name          string
		b             []byte
		cursorPos     int
		want          []byte
		wantCursorPos int
	}{
		{
			"Window title, cursor after",
			[]byte("\u001B]0;C:\\Users\\RUNNER~1\\AppData\\Local\\Temp\\2642502767\\cache\\94dd3fa4\\exec\\python3.exe\u0007Hello"),
			86, // First two characters of Hello
			[]byte("Hello"),
			2,
		},
		{
			"Window title, cursor preceding",
			[]byte("Hello\u001B]0;C:\\Users\\RUNNER~1\\AppData\\Local\\Temp\\2642502767\\cache\\94dd3fa4\\exec\\python3.exe\u0007World"),
			1, // First two characters of Hello
			[]byte("HelloWorld"),
			1,
		},
		{
			"Window title, cursor on top",
			[]byte("Hello\u001B]0;C:\\Users\\RUNNER~1\\AppData\\Local\\Temp\\2642502767\\cache\\94dd3fa4\\exec\\python3.exe\u0007World"),
			10, // Inside title escape sequence
			[]byte("HelloWorld"),
			4,
		},
		{
			"Backspace character",
			[]byte("Foo \u0008Bar"),
			7, // End of string
			[]byte("FooBar"),
			5,
		},
		{
			"Backspace character, cursor on top of backspace",
			[]byte("Foo \u0008Bar"),
			5, // End of string
			[]byte("FooBar"),
			3,
		},
		{
			"Cursor position preceding cleaned sequence",
			[]byte("Foo\u001B[1mBar"), // \u001B[1m = bold
			2,                         // End of "Foo"
			[]byte("FooBar"),
			2,
		},
		{
			"Cursor position succeeding cleaned sequence",
			[]byte("Foo\u001B[1mBar"), // \u001B[1m = bold
			9,                         // End of "Bar"
			[]byte("FooBar"),
			5,
		},
		{
			"Cursor position on top of cleaned sequence",
			[]byte("Foo\u001B[1mBar"), // \u001B[1m = bold
			4,                         // Unicode code point
			[]byte("FooBar"),
			2,
		},
		{
			"Negative cursor position",
			[]byte("Foo\u001B[1mBar"), // \u001B[1m = bold
			-10,                       // End of "Foo"
			[]byte("FooBar"),
			-10,
		},
		{
			// Running on ANSI escape codes obviously is not the intent, but without being able to easily identify
			// which is which this can be error-prone so we need to ensure this doesn't cause panics
			"Doesnt break if running on ANSI escape codes",
			[]byte("25h    25l █ Installing Runtime (Unconfigured) 25h 25l █25h 25l █ Installing Runtime Environment 25h 25l Setting Up Runtime       \n  Resolving Dependencies | 25h"),
			165,
			[]byte("25h    25l █ Installing Runtime (Unconfigured)25h 25l █25h 25l █ Installing Runtime Environment25h 25l Setting Up Runtime       \n  Resolving Dependencies |25h"),
			159,
		},
		{
			"Escape at first character",
			[]byte("\u001B[1mfoo"),
			0,
			[]byte("foo"),
			0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cursorPos > len(tt.b) {
				t.Fatal("cursor position cannot be larger than input")
			}
			if tt.wantCursorPos > len(tt.want) {
				t.Fatal("Wanted cursor position cannot be larger than wanted output")
			}
			cleaned, cursorPos := cleanPtySnapshot(tt.b, tt.cursorPos, false)
			assert.Equal(t, string(tt.want), string(cleaned))
			assert.Equal(t, tt.wantCursorPos, cursorPos)
		})
	}
}
