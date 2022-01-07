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
		tcpAddr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(cc.String(host), strconv.FormatUint(cc.Uint64(port), 10)))
		if err != nil {
			return err
		}

		chunkSize, err := units.FromHumanSize(cc.String(transferRate))
		if err != nil {
			return err
		}

		connexionTimeout := cc.Duration(connTimeout)
		if connexionTimeout == 0 {
			connexionTimeout = defaultClientConnTimeout
		}

		var options []grpc.DialOption
		if cc.Bool(connInsecure) {
			options = append(options, grpc.WithTransportCredentials(insecure.NewCredentials()))
		} else {
			var ca, cert, key []byte
			if cc.String(tlsCert) == "" {
				return errors.New("tls certificate is required in secure connection mode")
			}

			if cc.String(tlsKey) == "" {
				return errors.New("tls certificate key is required in secure connection mode")
			}

			if cc.String(tlsCa) != "" {
				ca, err = os.ReadFile(cc.String(tlsCa))
				if err != nil {
					return fmt.Errorf("unable to read root certificate: %w", err)
				}
			}

			cert, err = os.ReadFile(cc.String(tlsCert))
			if err != nil {
				return fmt.Errorf("unable to read certificate: %w", err)
			}

			key, err = os.ReadFile(cc.String(tlsKey))
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
			tcpAddr.String(),
			options...,
		)
		if err != nil {
			return err
		}
		defer conn.Close()

		pasteTimeout := cc.Duration(timeout)
		var pasteCtx context.Context
		var pasteCancel context.CancelFunc
		if pasteTimeout == 0 {
			pasteCtx, pasteCancel = context.WithCancel(context.Background())
		} else {
			pasteCtx, pasteCancel = context.WithTimeout(context.Background(), pasteTimeout)
		}
		defer pasteCancel()

		var stdout io.Writer = os.Stdout
		if cc.Bool(discard) {
			stdout = io.Discard
		}

		c := client.New(conn)

		sig := make(chan os.Signal, 2)
		pastErr := make(chan error)
		signal.Notify(sig, os.Interrupt, os.Kill)
		go func() {
			pastErr <- c.Paste(
				pasteCtx,
				stdout,
				pasteopt.WithPassword(cc.String(password)),
				pasteopt.WithTransferRate(chunkSize),
			)
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
			beeep.Notify("pasted", "this is looking absoluty awesome", "")
		}

		return nil
	}
}
