package command

import (
	crypto "crypto/rand"
	"encoding/binary"
	"fmt"
	"github.com/mattn/go-tty"
	vtgrpc "github.com/planetscale/vtprotobuf/codec/grpc"
	"github.com/tigerwill90/webcb/server"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc/encoding"
	_ "google.golang.org/grpc/encoding/proto"
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
	encoding.RegisterCodec(vtgrpc.Codec{})
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
	secretEnv  = "WEBCB_SECRET"
	hostEnv    = "WEBCB_HOST"
	portEnv    = "WEBCB_PORT"
	tlsCertEnv = "WEBCB_TLS_CERT"
	tlsKeyEnv  = "WEBCB_TLS_KEY"
	tlsCaEnv   = "WEBCB_TLS_ROOT_CA"
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
				Aliases: []string{"address"},
				Usage:   "the address of the Webcb server",
			},
			&cli.Uint64Flag{
				Name:    portFlag,
				Value:   4444,
				EnvVars: []string{portEnv},
				Usage:   "the port of the Webcb server",
			},
			&cli.StringFlag{
				Name:    tlsCertFlag,
				EnvVars: []string{tlsCertEnv},
				Usage:   "path to a PEM encoded client/server certificate for TLS authentication to the Webcb server",
			},
			&cli.StringFlag{
				Name:    tlsKeyFlag,
				EnvVars: []string{tlsKeyEnv},
				Usage:   "path to a PEM encoded client/server private key matching the client/server certificate",
			},
			&cli.StringFlag{
				Name:    tlsCaFlag,
				EnvVars: []string{tlsCaEnv},
				Usage:   "path to a PEM encoded CA certificate to use to verify the client/server SSL certificate",
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
						Usage: "timeout after which the initial TCP connection is considered to have failed",
					},
					&cli.BoolFlag{
						Name:  connInsecureFlag,
						Usage: "establish a connection without TLS (server dev mode only)",
					},
					&cli.DurationFlag{
						Name:        timeoutFlag,
						DefaultText: "0s - no timeout",
						Usage:       "timeout after which the status request is considered to have failed",
					},
				},
				Action: newStatusCommand(ui).run(),
			},
			{
				Name:    "serve",
				Aliases: []string{"s"},
				Usage:   "Start the Webcb server",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  devModeFlag,
						Usage: "start a wpc server in dev mode",
					},
					&cli.IntFlag{
						Name:  grpcMaxReceivedBytesFlag,
						Value: server.DefaultGrpcMaxRecvSize,
						Usage: "set the maximum bytes limit that the serve is able to handle per message",
					},
					&cli.DurationFlag{
						Name:  gcIntervalFlag,
						Value: 1 * time.Minute,
						Usage: "set the interval of the internal garbage collector",
					},
					&cli.StringFlag{
						Name:  pathFlag,
						Usage: "path to a location where the clipboard data is persisted",
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
						Usage:   "set the length limit per chunk sent to the server",
					},
					&cli.BoolFlag{
						Name:    watchFlag,
						Aliases: []string{"w"},
						Usage:   "not implemented yet",
					},
					&cli.DurationFlag{
						Name:        timeoutFlag,
						DefaultText: "0s - no timeout",
						Usage:       "timeout after which the copy request is considered to have failed",
					},
					&cli.DurationFlag{
						Name:  connTimeoutFlag,
						Value: defaultClientConnTimeout,
						Usage: "timeout after which the initial TCP connection is considered to have failed",
					},
					&cli.BoolFlag{
						Name:    checksumFlag,
						Aliases: []string{"sum"},
						Usage:   "enable checksum verification",
					},
					&cli.DurationFlag{
						Name:  ttlFlag,
						Value: server.DefaultTtl,
						Usage: "set the time to live of the copied data",
					},
					&cli.BoolFlag{
						Name:  verboseFlag,
						Usage: "print a summary after a successful copy",
					},
					&cli.BoolFlag{
						Name:  compressFlag,
						Usage: "enable data compression",
					},
					&cli.BoolFlag{
						Name:    noPasswordFlag,
						Aliases: []string{"nopass"},
						Usage:   "disable data encryption",
					},
					&cli.BoolFlag{
						Name:  connInsecureFlag,
						Usage: "establish a connection without TLS (server dev mode only)",
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
						Usage:   "set the length limit per chunk received from the server",
					},
					&cli.DurationFlag{
						Name:        timeoutFlag,
						DefaultText: "0s - no timeout",
						Usage:       "timeout after which the paste request is considered to have failed",
					},
					&cli.DurationFlag{
						Name:  connTimeoutFlag,
						Value: defaultClientConnTimeout,
						Usage: "timeout after which the initial TCP connection is considered to have failed",
					},
					&cli.BoolFlag{
						Name:  discardFlag,
						Usage: "discard the clipboard stream output (for testing purpose)",
					},
					&cli.BoolFlag{
						Name:  connInsecureFlag,
						Usage: "establish a connection without TLS (server dev mode only)",
					},
					&cli.StringFlag{
						Name:    fileFlag,
						Aliases: []string{"f"},
						Usage:   "path to a file where the clipboard data is pasted",
					},
					&cli.BoolFlag{
						Name:    clipboardFlag,
						Aliases: []string{"cb"},
						Usage:   "no implemented yet",
					},
				},
				Action: newPasteCommand().run(),
			},
			{
				Name:  "clear",
				Usage: "Clear the web clipboard",
				Flags: []cli.Flag{
					&cli.DurationFlag{
						Name:  connTimeoutFlag,
						Value: defaultClientConnTimeout,
						Usage: "timeout after which the initial TCP connection is considered to have failed",
					},
					&cli.BoolFlag{
						Name:  connInsecureFlag,
						Usage: "establish a connection without TLS (server dev mode only)",
					},
				},
				Action: newClearCommand().run(),
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
