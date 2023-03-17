package main

import (
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
	c := exec.Command("bash")

	// Start the command with a pty.
	ptmx, err := pty.StartWithSize(c, &pty.Winsize{Cols: 1000, Rows: 1})
	if err != nil {
		return err
	}
	// Make sure to close the pty at the end.
	defer func() { _ = ptmx.Close() }() // Best effort.

	go func() {
		time.Sleep(time.Second)
		ptmx.Write([]byte("echo hello\n"))
		ptmx.Write([]byte("exit\n"))
	}()

	for {
		select {
		case <-time.After(100 * time.Millisecond):
			buffer := make([]byte, 1024)
			n, err := ptmx.Read(buffer)
			if n > 0 {
				fmt.Printf("Buffer received: %s", string(buffer[:n]))
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

func readViaLoop(ptmx pty.Pty) {
	buffer := []byte{}
	for {
		bufferAppend := make([]byte, 1)
		err := readBytes(ptmx, bufferAppend)
		if len(bufferAppend) != 0 {
			buffer = append(buffer, bufferAppend...)
		}
		if err != nil {
			fmt.Println("Error: " + err.Error())
			break
		}
	}

	fmt.Printf("Buffer: %s", string(buffer))
}

// Will fill p from reader r
func readBytes(r io.Reader, p []byte) error {
	bytesRead := 0
	for bytesRead < len(p) {
		n, err := r.Read(p[bytesRead:])
		if err != nil {
			return err
		}
		bytesRead = bytesRead + n
	}
	return nil
}

func main() {
	if err := test(); err != nil {
		log.Fatal(err)
	}
}
