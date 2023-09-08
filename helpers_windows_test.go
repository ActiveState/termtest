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
			"Window title",
			[]byte("\u001B]0;C:\\Users\\RUNNER~1\\AppData\\Local\\Temp\\2642502767\\cache\\94dd3fa4\\exec\\python3.exe\u0007Hello"),
			96, // First two characters of Hello
			[]byte("Hello"),
			2,
		},
		{
			"Backspace character",
			[]byte("Foo \u0008Bar"),
			7, // End of string
			[]byte("FooBar"),
			6,
		},
		{
			"Cursor position preseding cleaned sequence",
			[]byte("Foo \u0008Bar"),
			3, // End of "Foo"
			[]byte("FooBar"),
			3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleaned, _ := cleanPtySnapshot(tt.b, 0, false)
			assert.Equalf(t, string(tt.want), string(cleaned), "cleanPtySnapshot(%v)", tt.b)
		})
	}
}
