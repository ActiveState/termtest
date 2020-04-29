// Copyright 2020 ActiveState Software, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file

package xpty

import (
	"bufio"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func prepare() (r *io.PipeReader, w *io.PipeWriter, p *PassthroughPipe, closer func()) {
	r, w = io.Pipe()
	br := bufio.NewReaderSize(r, 100)
	p = NewPassthroughPipe(br)
	return r, w, p, func() {
		_ = r.Close()
		_ = w.Close()
		_ = p.Close()
	}
}

func TestPassthroughPipeCustomError(t *testing.T) {
	_, w, p, close := prepare()
	defer close()

	p.SetReadDeadline(time.Now().Add(time.Second * 2))

	pipeError := errors.New("pipe error")
	err := w.CloseWithError(pipeError)
	require.NoError(t, err)

	_, sz, err := p.ReadRune()
	require.Equal(t, 0, sz)
	require.Equal(t, pipeError, err)
}

func TestPassthroughPipeEOFError(t *testing.T) {
	_, w, p, close := prepare()
	defer close()

	p.SetReadDeadline(time.Now().Add(time.Second * 2))

	err := w.Close()
	require.NoError(t, err)

	_, sz, err := p.ReadRune()
	require.Equal(t, 0, sz)
	require.Equal(t, io.EOF, err)
}

func readString(p *PassthroughPipe, n int) (string, int, error) {
	var total int
	s := make([]rune, 0, n)
	for i := 0; i < n; i++ {
		r, sz, err := p.ReadRune()
		if err != nil {
			return string(s), total, err
		}
		if sz == 0 {
			continue
		}
		total += sz
		s = append(s, r)
	}
	return string(s), total, nil
}

func TestPassthroughPipe(t *testing.T) {
	_, w, p, close := prepare()
	defer close()

	p.SetReadDeadline(time.Now().Add(time.Second * 2))

	go func() {
		_, err := w.Write([]byte("12abc"))
		require.NoError(t, err, "writing bytes")
		err = w.Close()
		require.NoError(t, err, "closing writer")
	}()

	res, n, err := readString(p, 2)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, "12", res)

	res, n, err = readString(p, 3)
	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.Equal(t, "abc", res)

	_, n, err = readString(p, 10)
	require.Error(t, err, io.EOF)
	require.Equal(t, 0, n)
}

// TestPassthroughPipeReadDrain drains the PassthroughPipe very slowly
// This is a regression test, ensuring that errors during reading from the pipe
// are processed *after* all bytes written to the pipe have been read
func TestPassthroughPipeReadDrain(t *testing.T) {
	_, w, p, close := prepare()
	defer close()

	p.SetReadDeadline(time.Now().Add(1 * time.Second))

	b := make([]byte, 100)
	for i := 0; i < 100; i++ {
		b[i] = byte(i)
	}

	go func() {
		n, err := w.Write(b)
		require.Equal(t, 100, n)
		require.NoError(t, err)
		err = w.Close()
		require.NoError(t, err, "closing writer")
	}()
	// pipewriter is concurrent; sleep to let buffer fill
	time.Sleep(10 * time.Millisecond)

	for i := 0; i < 100; i++ {
		res, n, err := readString(p, 1)
		require.NoError(t, err)
		require.Equal(t, 1, n)
		require.Equal(t, byte(i), byte(res[0]))
	}
	_, n, err := p.ReadRune()
	require.Error(t, err, io.EOF)
	require.Equal(t, 0, n)
}

func TestPassthroughPipeReadAfterClose(t *testing.T) {
	_, w, p, cleanup := prepare()
	defer cleanup()

	p.SetReadDeadline(time.Now().Add(200 * time.Millisecond))

	go func() {
		w.Write([]byte("abc"))
		w.Close()
	}()

	res, n, err := readString(p, 4)
	require.Equal(t, io.EOF, err)
	require.Equal(t, 3, n)
	require.Equal(t, "abc", res)

	_, n, err = p.ReadRune()
	require.Error(t, err)
	require.Equal(t, 0, n)
	require.Equal(t, io.EOF, err)

	p.Close()

	_, n, err = p.ReadRune()
	require.Error(t, err)
	require.Equal(t, 0, n)
	require.Equal(t, io.EOF, err)
}

func TestPassthroughPipeTimeout(t *testing.T) {
	_, w, p, close := prepare()
	defer close()

	p.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

	go func() {
		_, err := w.Write([]byte("abc"))
		require.NoError(t, err, "writing test string")
		err = w.Close()
		require.NoError(t, err, "closing writer")
	}()

	res, n, err := readString(p, 3)
	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.Equal(t, "abc", res)

	_, n, err = p.ReadRune()
	require.Equal(t, 0, n)
	require.Error(t, err, "i/o deadline exceeded")
}
