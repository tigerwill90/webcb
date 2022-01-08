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

const (
	host                 = "host"
	port                 = "port"
	tlsCert              = "cert"
	tlsKey               = "key"
	tlsCa                = "ca"
	connInsecure         = "insecure"
	connTimeout          = "conn-timeout"
	devMode              = "dev"
	grpcMaxReceivedBytes = "grpc-max-receive-bytes"
	gcInterval           = "gc-interval"
	dbPath               = "path"
	timeout              = "timeout"
	transferRate         = "transfer-rate"
	checksum             = "checksum"
	ttl                  = "ttl"
	verbose              = "verbose"
	compress             = "compress"
	password             = "password"
	discard              = "discard"
)

func Run(args []string) int {

	ui := newUi(os.Stdout, os.Stderr)

	app := &cli.App{
		Name:        "webcb",
		Usage:       "the web clipboard",
		Description: "Clipboard copy/paste over internet",
		Version:     "v0.0.0",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  host,
				Value: "0.0.0.0",
			},
			&cli.Uint64Flag{
				Name:  port,
				Value: 4444,
			},
			&cli.StringFlag{
				Name: tlsCert,
			},
			&cli.StringFlag{
				Name: tlsKey,
			},
			&cli.StringFlag{
				Name: tlsCa,
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "status",
				Usage: "Show server status",
				Flags: []cli.Flag{
					&cli.DurationFlag{
						Name:  connTimeout,
						Value: defaultClientConnTimeout,
					},
					&cli.BoolFlag{
						Name: connInsecure,
					},
					&cli.DurationFlag{
						Name:        timeout,
						DefaultText: "0s - no timeout",
					},
				},
				Action: newStatusCommand().run(),
			},
			{
				Name:    "serve",
				Aliases: []string{"s"},
				Usage:   "Run a webcb server",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  devMode,
						Usage: "start a wpc server in dev mode",
					},
					&cli.IntFlag{
						Name:  grpcMaxReceivedBytes,
						Value: server.DefaultGrpcMaxRecvSize,
					},
					&cli.DurationFlag{
						Name:  gcInterval,
						Value: 3 * time.Minute,
					},
					&cli.StringFlag{
						Name:  dbPath,
						Value: "db",
					},
				},
				Action: newServerCmd().run(),
			},
			{
				Name:    "copy",
				Aliases: []string{"c"},
				Usage:   "Copy to web clipboard",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    transferRate,
						Aliases: []string{"rate"},
						Value:   "1048576b",
					},
					&cli.DurationFlag{
						Name:        timeout,
						DefaultText: "0s - no timeout",
					},
					&cli.DurationFlag{
						Name:  connTimeout,
						Value: defaultClientConnTimeout,
					},
					&cli.BoolFlag{
						Name:    checksum,
						Aliases: []string{"sum"},
					},
					&cli.DurationFlag{
						Name:  ttl,
						Value: server.DefaultTtl,
					},
					&cli.BoolFlag{
						Name: verbose,
					},
					&cli.BoolFlag{
						Name: compress,
					},
					&cli.StringFlag{
						Name:    password,
						Aliases: []string{"pwd"},
					},
					&cli.BoolFlag{
						Name: connInsecure,
					},
				},
				Action: newCopyCommand(ui).run(),
			},
			{
				Name:    "paste",
				Aliases: []string{"p"},
				Usage:   "Paste from web clipboard",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    transferRate,
						Aliases: []string{"rate"},
						Value:   "1048576b",
					},
					&cli.DurationFlag{
						Name:        timeout,
						DefaultText: "0s - no timeout",
					},
					&cli.DurationFlag{
						Name:  connTimeout,
						Value: defaultClientConnTimeout,
					},
					&cli.BoolFlag{
						Name: verbose,
					},
					&cli.BoolFlag{
						Name:  discard,
						Usage: "discard the clipboard stream output (for testing purpose)",
					},
					&cli.StringFlag{
						Name:    password,
						Aliases: []string{"pwd"},
					},
					&cli.BoolFlag{
						Name: connInsecure,
					},
				},
				Action: newPasteCommand().run(),
			},
			{
				Name:  "clean",
				Usage: "Clear the clipboard",
				Flags: []cli.Flag{
					&cli.DurationFlag{
						Name:  connTimeout,
						Value: defaultClientConnTimeout,
					},
					&cli.BoolFlag{
						Name: connInsecure,
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
