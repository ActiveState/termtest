package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

func main() {
	total, err := strconv.Atoi(os.Args[1])
	if err != nil {
		panic(err)
	}
	for x := 0; x < total; x++ {
		reader := bufio.NewReader(os.Stdin)

		fmt.Printf("Confirm %d?\n", x)

		response, err := reader.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}

		response = strings.ToLower(strings.TrimSpace(response))

		v := false
		if response == "y" {
			v = true
		}

		fmt.Printf("Answer %d: %v\n", x, v)
	}
}
