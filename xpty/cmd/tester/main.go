package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"

	"github.com/ActiveState/termtest/conpty"
	"golang.org/x/crypto/ssh/terminal"
)

func main() {
	// ENABLE_VIRTUAL_TERMINAL_PROCESSING on windows
	reset, err := conpty.InitTerminal(false)
	if err != nil {
		log.Fatalf("Could not enable virtual terminal processing: %v", err)
	}
	defer reset()

	fmt.Printf("1   5    10   15   20   25    30   35   40\n2\n3\n4\n5        \033[6n<cursor>hello world\n")

	// set the terminal into raw mode so we can receive the cursor position without waiting for a newline
	oldState, err := terminal.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf("Could not put terminal in raw mode: %v\n", err)
	}
	defer terminal.Restore(0, oldState)

	buf := make([]byte, 100)
	n, err := os.Stdin.Read(buf)
	if err != nil {
		fmt.Printf("Error reading from stdin: %v\n", err)
	}

	cpr := regexp.MustCompile(`.(\d+);(\d+)R`)
	matches := cpr.FindSubmatch(buf[:n])
	if len(matches) == 0 {
		log.Fatal("Could not find the cursor position")
	}

	row, _ := strconv.Atoi(string(matches[1]))
	col, _ := strconv.Atoi(string(matches[2]))
	fmt.Printf("cursor is at row %d and col %d\n", row, col)

}
