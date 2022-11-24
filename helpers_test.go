package termtest

import (
	"io"
	"time"
)

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
