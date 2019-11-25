package main

import (
	"os"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/OnitiFR/mulch/cmd/mulch/topics"
)

func main() {

	client.InitExitMessage()

	err := topics.Execute()

	msg := client.GetExitMessage()
	msg.Display()

	if err != nil {
		os.Exit(1)
	}
}
