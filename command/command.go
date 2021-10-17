package command

import (
	crypto "crypto/rand"
	"encoding/binary"
	"fmt"
	"github.com/urfave/cli/v2"
	"math/rand"
	"os"
)

func init() {
	var b [8]byte
	_, err := crypto.Read(b[:])
	if err != nil {
		panic(err)
	}
	rand.Seed(int64(binary.LittleEndian.Uint64(b[:])))
}

func Run(args []string) int {
	app := &cli.App{
		Name:        "webcb",
		Usage:       "the web clipboard",
		Description: "Clipboard copy/paste over internet",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "host",
				Value: "127.0.0.1",
			},
			&cli.Uint64Flag{
				Name:  "port",
				Value: 4444,
			},
		},
		Commands: []*cli.Command{
			{
				Name:    "serve",
				Aliases: []string{"s"},
				Usage:   "run a webcb server",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "dev",
						Usage: "start a wpc server in dev mode",
					},
				},
				Action: newServerCmd().run(),
			},
			{
				Name:    "copy",
				Aliases: []string{"cp"},
				Usage:   "copy to web clipboard",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "size",
						Value: "1048576b",
					},
					&cli.DurationFlag{
						Name: "timeout",
					},
					&cli.DurationFlag{
						Name:  "client-timeout",
						Value: defaultClientTimeout,
					},
					&cli.BoolFlag{
						Name: "integrity",
					},
				},
				Action: newClipboardCmd().run(),
			},
		},
	}

	if err := app.Run(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	return 0
}
