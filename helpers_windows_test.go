package termtest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_cleanPtySequences(t *testing.T) {
	tests := []struct {
		name string
		b    []byte
		want []byte
	}{
		{
			"Window title",
			[]byte("\u001B]0;C:\\Users\\RUNNER~1\\AppData\\Local\\Temp\\2642502767\\cache\\94dd3fa4\\exec\\python3.exe\u0007Hello"),
			[]byte("Hello"),
		},
		{
			"Backspace character",
			[]byte("Foo \u0008Bar"),
			[]byte("FooBar"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, string(tt.want), string(cleanPtySnapshot(tt.b, false)), "cleanPtySnapshot(%v)", tt.b)
		})
	}
}
