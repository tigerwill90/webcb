package command

import (
	"fmt"
	"github.com/urfave/cli/v2"
	"os"
)

func Run(args []string) int {
	app := &cli.App{
		Name:        "wcp",
		Usage:       "the web copy tool",
		Description: "Clipboard copy/paste over internet",
		Commands: []*cli.Command{
			{
				Name:    "server",
				Aliases: []string{"s"},
				Usage:   "run a wpc server",
				Action:  newServer().run(),
			},
		},
	}

	if err := app.Run(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	return 0
}
