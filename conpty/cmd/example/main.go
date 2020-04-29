// Copyright 2020 ActiveState Software. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file

package main

// Note: This works only on Windows

import (
	"fmt"
	"log"
	"os"
	"syscall"
	"time"

	"github.com/ActiveState/termtest/conpty"
)

func main() {
	cpty, err := conpty.New(80, 25)
	if err != nil {
		log.Fatalf("Could not open a conpty terminal: %v", err)
	}
	defer cpty.Close()

	pid, _, err := cpty.Spawn(
		"C:\\WINDOWS\\System32\\WindowsPowerShell\\v1.0\\powershell.exe",
		[]string{},
		&syscall.ProcAttr{
			Env: os.Environ(),
		},
	)
	if err != nil {
		log.Fatalf("Could not spawn a powershell: %v", err)
	}
	fmt.Printf("New process with pid %d spawned\n", pid)

	process, err := os.FindProcess(pid)
	if err != nil {
		log.Fatalf("Failed to find process: %v", err)
	}
	defer func() {
		ps, err := process.Wait()
		if err != nil {
			log.Fatalf("Error waiting for process: %v", err)
		}
		fmt.Printf("exit code was: %d\n", ps.ExitCode())
	}()

	cpty.Write([]byte("echo \"hello world\"\r\n"))
	if err != nil {
		log.Fatalf("Failed to write to conpty: %v", err)
	}

	// Give powershell some time to start
	time.Sleep(5 * time.Second)

	buf := make([]byte, 5000)
	n, err := cpty.OutPipe().Read(buf)
	if err != nil {
		log.Fatalf("Failed to read from conpty: %v", err)
	}
	fmt.Printf("Terminal output:\n%s\n", string(buf[:n]))
	cpty.Write([]byte("exit\r\n"))
	if err != nil {
		log.Fatalf("Failed to write to conpty: %v", err)
	}
}
