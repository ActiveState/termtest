package main

import (
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
)

func main() {
	// perform the questions
	var answer bool
	err := survey.AskOne(&survey.Confirm{
		Message: "Confirm?",
	}, &answer, nil)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
		return
	}
}
