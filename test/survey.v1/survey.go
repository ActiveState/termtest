package main

import (
	"fmt"
	"os"

	"gopkg.in/AlecAivazis/survey.v1"
	"gopkg.in/AlecAivazis/survey.v1/core"
)

func main() {
	fmt.Println("Start")
	defer fmt.Println("End")
	// perform the questions
	var answer bool
	core.DisableColor = true
	err := survey.AskOne(&survey.Confirm{
		Message: "Confirm?",
	}, &answer, nil)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
		return
	}
	fmt.Printf("Result: %v\n", answer)
}
