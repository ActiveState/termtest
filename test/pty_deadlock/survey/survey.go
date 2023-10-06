package main

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/AlecAivazis/survey.v1"
)

func main() {
	total, err := strconv.Atoi(os.Args[1])
	if err != nil {
		panic(err)
	}
	for x := 0; x < total; x++ {
		// perform the questions
		var answer bool
		err := survey.AskOne(&survey.Confirm{
			Message: fmt.Sprintf("Confirm %d?", x),
		}, &answer, nil)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
			return
		}
		fmt.Printf("Answer %d: %v\n", x, answer)
	}
}
