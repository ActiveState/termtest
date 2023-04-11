package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os/exec"
	"time"

	"github.com/creack/pty"
)

func test() error {
	// Create arbitrary command.
	c := exec.Command("cmd")

	// Start the command with a pty.
	ptmx, err := pty.StartWithSize(c, &pty.Winsize{Cols: 1000, Rows: 10})
	if err != nil {
		return err
	}
	// Make sure to close the pty at the end.
	defer func() { _ = ptmx.Close() }() // Best effort.

	go func() {
		time.Sleep(time.Second)
		ptmx.Write([]byte("echo helloooooooooooo!\r\n"))
		ptmx.Write([]byte("exit\r\n"))
	}()

	for {
		select {
		case <-time.After(100 * time.Millisecond):
			fmt.Println("Reading..")
			buffer := make([]byte, 20)
			n, err := ptmx.Read(buffer)
			buffer = cleanBytes(buffer)
			fmt.Printf("Read %d bytes\n", n)
			if n > 0 {
				fmt.Printf("Buffer received: %s\n", string(buffer[:n]))
			}

			// Error doesn't necessarily mean something went wrong, we may just have reached the natural end
			if err != nil {
				if errors.Is(err, fs.ErrClosed) || errors.Is(err, io.EOF) {
					fmt.Println("End reached")
					ps, err := c.Process.Wait()
					if err != nil {
						return err
					}
					fmt.Printf("Process state: %#v | %#v", ps.ExitCode(), c.ProcessState)
					return nil
				}
				return fmt.Errorf("could not read pty output: %w", err)
			}
		}
	}

	return nil
}

func main() {
	if err := test(); err != nil {
		log.Fatal(err)
	}
}

// cleanBytes removes virtual escape sequences from the given byte slice
// https://learn.microsoft.com/en-us/windows/console/console-virtual-terminal-sequences
func cleanBytes(b []byte) (o []byte) {
	// All escape sequences appear to end on `A-Za-z@`
	virtualEscapeSeqEndValues := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz@")
	inEscapeSequence := false

	return bytes.Map(func(r rune) rune {

		// Detect start of sequence
		if r == 27 {
			inEscapeSequence = true
			return -1
		}

		// Detect end of sequence
		if inEscapeSequence && bytes.ContainsRune(virtualEscapeSeqEndValues, r) {
			inEscapeSequence = false
			return -1
		}

		// Anything between start and end of escape sequence should also be dropped
		if inEscapeSequence {
			return -1
		}

		return r
	}, b)
}
