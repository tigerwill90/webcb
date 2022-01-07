package main

import (
	"github.com/tigerwill90/webcb/command"
	"os"
)

func main() {
	os.Exit(command.Run(os.Args))
}
