package termtest

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

type readTesterChunk struct {
	bytes []byte
	delay time.Duration
}

type readTester struct {
	chunks []readTesterChunk
}

func (r *readTester) Read(p []byte) (int, error) {
	if len(r.chunks) == 0 {
		return 0, io.EOF
	}
	chunk := r.chunks[0]
	time.Sleep(chunk.delay)
	n := copy(p, chunk.bytes)
	if len(chunk.bytes) > n {
		r.chunks[0].bytes = chunk.bytes[n:]
	} else {
		r.chunks = append(r.chunks[:0], r.chunks[1:]...)
	}
	return n, nil
}

func newTestOpts(o *Opts, t *testing.T) *Opts {
	if o == nil {
		o = &Opts{}
	}
	o.Logger = log.New(os.Stderr, filepath.Base(t.Name())+": ", log.Ltime|log.Lmicroseconds|log.Lshortfile)
	o.ExpectErrorHandler = func(t *TermTest, err error) error {
		return fmt.Errorf("Error encountered: %w\nOutput: %s", err, t.Output())
	}
	return o
}

func newTermTest(t *testing.T, cmd *exec.Cmd, logging bool, opts ...SetOpt) *TermTest {
	tt, err := New(cmd, append(opts, func(o *Opts) error {
		o = newTestOpts(o, t)
		if !logging {
			o.Logger = VoidLogger
		}
		return nil
	}, OptTestErrorHandler(t))...)
	require.NoError(t, err)
	return tt
}

func randString(n int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func indexEndPos(s, substr string) int {
	i := strings.Index(s, substr)
	if i == -1 {
		return 0
	}
	return i + len(substr)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

type BlackholeWriter struct {
}

func (BlackholeWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func toPosixPath(p string) string {
	return regexp.MustCompile(`^([A-Za-z])\:`).ReplaceAllString(filepath.ToSlash(p), "/$1")
}
