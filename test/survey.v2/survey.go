package main

import (
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
)

func main() {
	for x := 0; x < 10; x++ {
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
