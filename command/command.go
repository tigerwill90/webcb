package command

import (
	crypto "crypto/rand"
	"encoding/binary"
	"fmt"
	"github.com/mattn/go-tty"
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
	hostFlag                 = "host"
	portFlag                 = "port"
	tlsCertFlag              = "cert"
	tlsKeyFlag               = "key"
	tlsCaFlag                = "ca"
	connInsecureFlag         = "insecure"
	connTimeoutFlag          = "conn-timeout"
	devModeFlag              = "dev"
	grpcMaxReceivedBytesFlag = "grpc-max-receive-bytes"
	gcIntervalFlag           = "gc-interval"
	pathFlag                 = "path"
	timeoutFlag              = "timeout"
	transferRateFlag         = "transfer-rate"
	checksumFlag             = "checksum"
	ttlFlag                  = "ttl"
	verboseFlag              = "verbose"
	compressFlag             = "compress"
	discardFlag              = "discard"
	fileFlag                 = "file"
	watchFlag                = "watch"
	noPasswordFlag           = "no-password"
	clipboardFlag            = "clipboard"
)

const (
	secretEnv = "WEBCB_SECRET"
	hostEnv   = "WEBCB_HOST"
	portEnv   = "WEBCB_PORT"
)

const (
	defaultServerAddr = "0.0.0.0"
	defaultClientAddr = "127.0.0.1"
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
				Name:    hostFlag,
				EnvVars: []string{hostEnv},
			},
			&cli.Uint64Flag{
				Name:    portFlag,
				Value:   4444,
				EnvVars: []string{portEnv},
			},
			&cli.StringFlag{
				Name: tlsCertFlag,
			},
			&cli.StringFlag{
				Name: tlsKeyFlag,
			},
			&cli.StringFlag{
				Name: tlsCaFlag,
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "status",
				Usage: "Show server status",
				Flags: []cli.Flag{
					&cli.DurationFlag{
						Name:  connTimeoutFlag,
						Value: defaultClientConnTimeout,
					},
					&cli.BoolFlag{
						Name: connInsecureFlag,
					},
					&cli.DurationFlag{
						Name:        timeoutFlag,
						DefaultText: "0s - no timeout",
					},
				},
				Action: newStatusCommand(ui).run(),
			},
			{
				Name:    "serve",
				Aliases: []string{"s"},
				Usage:   "Run a webcb server",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  devModeFlag,
						Usage: "start a wpc server in dev mode",
					},
					&cli.IntFlag{
						Name:  grpcMaxReceivedBytesFlag,
						Value: server.DefaultGrpcMaxRecvSize,
					},
					&cli.DurationFlag{
						Name:  gcIntervalFlag,
						Value: 1 * time.Minute,
					},
					&cli.StringFlag{
						Name: pathFlag,
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
						Name:    transferRateFlag,
						Aliases: []string{"rate"},
						Value:   "1048576b",
					},
					&cli.BoolFlag{
						Name:    watchFlag,
						Aliases: []string{"w"},
					},
					&cli.DurationFlag{
						Name:        timeoutFlag,
						DefaultText: "0s - no timeout",
					},
					&cli.DurationFlag{
						Name:  connTimeoutFlag,
						Value: defaultClientConnTimeout,
					},
					&cli.BoolFlag{
						Name:    checksumFlag,
						Aliases: []string{"sum"},
					},
					&cli.DurationFlag{
						Name:  ttlFlag,
						Value: server.DefaultTtl,
					},
					&cli.BoolFlag{
						Name: verboseFlag,
					},
					&cli.BoolFlag{
						Name: compressFlag,
					},
					&cli.BoolFlag{
						Name:    noPasswordFlag,
						Aliases: []string{"nopass"},
					},
					&cli.BoolFlag{
						Name: connInsecureFlag,
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
						Name:    transferRateFlag,
						Aliases: []string{"rate"},
						Value:   "1048576b",
					},
					&cli.DurationFlag{
						Name:        timeoutFlag,
						DefaultText: "0s - no timeout",
					},
					&cli.DurationFlag{
						Name:  connTimeoutFlag,
						Value: defaultClientConnTimeout,
					},
					&cli.BoolFlag{
						Name: verboseFlag,
					},
					&cli.BoolFlag{
						Name:  discardFlag,
						Usage: "discard the clipboard stream output (for testing purpose)",
					},
					&cli.BoolFlag{
						Name: connInsecureFlag,
					},
					&cli.StringFlag{
						Name:    fileFlag,
						Aliases: []string{"f"},
					},
					&cli.BoolFlag{
						Name:    clipboardFlag,
						Aliases: []string{"cb"},
					},
				},
				Action: newPasteCommand().run(),
			},
			{
				Name:  "clean",
				Usage: "Clear the clipboard",
				Flags: []cli.Flag{
					&cli.DurationFlag{
						Name:  connTimeoutFlag,
						Value: defaultClientConnTimeout,
					},
					&cli.BoolFlag{
						Name: connInsecureFlag,
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

func readSecret() ([]byte, error) {
	pwd := os.Getenv(secretEnv)
	if pwd != "" {
		return []byte(pwd), nil
	}

	tty, err := tty.Open()
	if err != nil {
		return nil, err
	}
	defer tty.Close()

	if _, err := fmt.Fprint(tty.Output(), "Password: "); err != nil {
		return nil, err
	}
	pwd, err = tty.ReadPasswordNoEcho()
	if err != nil {
		return nil, err
	}
	return []byte(pwd), nil
}
