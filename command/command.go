package command

import (
	crypto "crypto/rand"
	"encoding/binary"
	"fmt"
	"github.com/tigerwill90/webcb/server"
	"github.com/urfave/cli/v2"
	"math/rand"
	"os"
	"time"
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
		Version:     "v0.0.0",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "host",
				Value: "0.0.0.0",
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
					&cli.IntFlag{
						Name:  "grpc-max-receive-bytes",
						Value: defaultGrpcMaxRecvBytes,
					},
					&cli.DurationFlag{
						Name:  "gc-interval",
						Value: 5 * time.Minute,
					},
					&cli.StringFlag{
						Name:  "path",
						Value: "db",
					},
				},
				Action: newServerCmd().run(),
			},
			{
				Name:    "copy",
				Aliases: []string{"c"},
				Usage:   "copy to web clipboard",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "size",
						Value: "1048576b",
					},
					&cli.DurationFlag{
						Name:        "timeout",
						DefaultText: "0s - no timeout",
					},
					&cli.DurationFlag{
						Name:  "conn-timeout",
						Value: defaultClientConnTimeout,
					},
					&cli.BoolFlag{
						Name:    "checksum",
						Aliases: []string{"sum"},
					},
					&cli.DurationFlag{
						Name:  "ttl",
						Value: server.DefaultTtl,
					},
					&cli.BoolFlag{
						Name: "verbose",
					},
					&cli.BoolFlag{
						Name: "compress",
					},
					&cli.StringFlag{
						Name:    "password",
						Aliases: []string{"pwd"},
					},
				},
				Action: newCopyCommand().run(),
			},
			{
				Name:    "paste",
				Aliases: []string{"p"},
				Usage:   "paste from web clipboard",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "size",
						Value: "1048576b",
					},
					&cli.DurationFlag{
						Name:        "timeout",
						DefaultText: "0s - no timeout",
					},
					&cli.DurationFlag{
						Name:  "conn-timeout",
						Value: defaultClientConnTimeout,
					},
					&cli.BoolFlag{
						Name: "verbose",
					},
					&cli.BoolFlag{
						Name:  "discard",
						Usage: "discard the clipboard stream output (for testing purpose)",
					},
					&cli.StringFlag{
						Name:    "password",
						Aliases: []string{"pwd"},
					},
				},
				Action: newPasteCommand().run(),
			},
			{
				Name: "clean",
				Flags: []cli.Flag{
					&cli.DurationFlag{
						Name:  "conn-timeout",
						Value: defaultClientConnTimeout,
					},
				},
				Action: newCleanCommand().run(),
			},
		},
	}

	if err := app.Run(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	return 0
}
