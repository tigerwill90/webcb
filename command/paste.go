package command

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/docker/go-units"
	"github.com/gen2brain/beeep"
	"github.com/tigerwill90/webcb/client"
	"github.com/tigerwill90/webcb/client/pasteopt"
	grpctls "github.com/tigerwill90/webcb/internal/tls"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
)

type pasteCmd struct{}

func newPasteCommand() *pasteCmd {
	return &pasteCmd{}
}

func (s *pasteCmd) run() cli.ActionFunc {
	return func(cc *cli.Context) error {
		host := cc.String(hostFlag)
		if host == "" {
			host = defaultClientAddr
		}

		address := net.JoinHostPort(host, strconv.FormatUint(cc.Uint64(portFlag), 10))

		chunkSize, err := units.FromHumanSize(cc.String(transferRateFlag))
		if err != nil {
			return err
		}

		connexionTimeout := cc.Duration(connTimeoutFlag)
		if connexionTimeout == 0 {
			connexionTimeout = defaultClientConnTimeout
		}

		var options []grpc.DialOption
		if cc.Bool(connInsecureFlag) {
			options = append(options, grpc.WithTransportCredentials(insecure.NewCredentials()))
		} else {
			var ca, cert, key []byte
			if cc.String(tlsCertFlag) == "" {
				return errors.New("tls certificate is required in secure connection mode")
			}

			if cc.String(tlsKeyFlag) == "" {
				return errors.New("tls certificate key is required in secure connection mode")
			}

			if cc.String(tlsCaFlag) != "" {
				ca, err = os.ReadFile(cc.String(tlsCaFlag))
				if err != nil {
					return fmt.Errorf("unable to read root certificate: %w", err)
				}
			}

			cert, err = os.ReadFile(cc.String(tlsCertFlag))
			if err != nil {
				return fmt.Errorf("unable to read certificate: %w", err)
			}

			key, err = os.ReadFile(cc.String(tlsKeyFlag))
			if err != nil {
				return fmt.Errorf("unable to read certificate key: %w", err)
			}

			tlsConfig := &tls.Config{}
			if err := grpctls.LoadCertificate(ca, cert, key, tlsConfig); err != nil {
				return err
			}

			options = append(options, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
		}

		ctx, cancel := context.WithTimeout(context.Background(), connexionTimeout)
		defer cancel()
		conn, err := grpc.DialContext(
			ctx,
			address,
			options...,
		)
		if err != nil {
			return err
		}
		defer conn.Close()

		pasteTimeout := cc.Duration(timeoutFlag)
		var pasteCtx context.Context
		var pasteCancel context.CancelFunc
		if pasteTimeout == 0 {
			pasteCtx, pasteCancel = context.WithCancel(context.Background())
		} else {
			pasteCtx, pasteCancel = context.WithTimeout(context.Background(), pasteTimeout)
		}
		defer pasteCancel()

		var stdout io.Writer = os.Stdout
		if path := cc.String(fileFlag); path != "" {
			var err error
			stdout, err = os.Create(path)
			if err != nil {
				return err
			}
		}
		if cc.Bool(discardFlag) {
			stdout = io.Discard
		}

		c := client.New(conn)

		sig := make(chan os.Signal, 2)
		pastErr := make(chan error)
		signal.Notify(sig, os.Interrupt, os.Kill)
		go func() {
			pastErr <- c.Paste(pasteCtx, stdout, client.Password(readSecret), pasteopt.WithTransferRate(chunkSize))
		}()

		select {
		case <-sig:
			return fmt.Errorf("paste canceled")
		case cErr := <-pastErr:
			if cErr != nil {
				if errors.Is(pasteCtx.Err(), context.DeadlineExceeded) {
					return fmt.Errorf("paste failed: %w", pasteCtx.Err())
				}
				return fmt.Errorf("paste failed: %w", cErr)
			}
		}

		return beeep.Notify("Paste success", "All data has been copied from remote clipboard", "")
	}
}
