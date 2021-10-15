package main

import (
	"github.com/awnumar/memguard"
	"github.com/tigerwill90/wcp/command"
	"os"
)

func main() {
	memguard.SafeExit(command.Run(os.Args))
}
