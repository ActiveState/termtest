// Copyright 2018 Netflix, Inc.
// Copyright 2020 ActiveState Software, Inc.
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
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync/atomic"
	"time"
)

type errPassthroughTimeout struct {
	error
}

func (errPassthroughTimeout) Timeout() bool { return true }

// PassthroughPipe pipes data from a io.Reader and allows setting a read
// deadline. If a timeout is reached the error is returned, otherwise the error
// from the provided io.Reader returned is passed through instead.
type PassthroughPipe struct {
	rdr      io.Reader
	deadline time.Time
	ctx      context.Context
	cancel   context.CancelFunc
	lastRead int64
	logFile  *os.File
}

// NewPassthroughPipe returns a new pipe for a io.Reader that passes through
// non-timeout errors.
func NewPassthroughPipe(r io.Reader) *PassthroughPipe {
	ctx, cancel := context.WithCancel(context.Background())
	f, err := os.OpenFile("text.log",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644,
	)
	if err != nil {
		log.Fatalf("Could not open log file: %v\n", err)
	}
	p := PassthroughPipe{
		rdr:      r,
		deadline: time.Now(),
		ctx:      ctx,
		cancel:   cancel,
		logFile:  f,
	}

	return &p
}

// IsBlocked returns true when the PassthroughPipe is (most likely) blocked reading ie., waiting for input
func (p *PassthroughPipe) IsBlocked() bool {
	lr := atomic.LoadInt64(&p.lastRead)
	return time.Duration(time.Now().UTC().UnixNano()-lr) > 500*time.Microsecond
}

// SetReadDeadline sets a deadline for a successful read
func (p *PassthroughPipe) SetReadDeadline(d time.Time) {
	p.deadline = d
}

// Close releases all resources allocated by the pipe
func (p *PassthroughPipe) Close() error {
	p.logFile.Close()
	p.cancel()
	return nil
}

type chunk struct {
	size int
	err  error
}

// Read reads from the PassthroughPipe and errors out if no data has been written to the pipe before the read deadline expired
// If read is called after the PassthroughPipe has been closed `0, io.EOF` is returned
func (p *PassthroughPipe) Read(buf []byte) (n int, err error) {
	cs := make(chan chunk)
	done := make(chan struct{})
	defer close(done)
	atomic.StoreInt64(&p.lastRead, time.Now().UTC().UnixNano())

	go func() {
		defer close(cs)

		if p.ctx.Err() != nil || p.deadline.Before(time.Now()) {
			return
		}

		n, err := p.rdr.Read(buf)

		m, werr := p.logFile.Write(buf[:n])
		if werr != nil {
			fmt.Printf("failed to write %d bytes to log file: %v\n", m, werr)
		}

		select {
		case <-done:
			return
		default:
			cs <- chunk{n, err}
		}
	}()

	select {
	case c := <-cs:
		return c.size, c.err

	case <-p.ctx.Done():
		return 0, io.EOF

	case <-time.After(p.deadline.Sub(time.Now())):
		return 0, &errPassthroughTimeout{errors.New("passthrough i/o timeout")}
	}
}
