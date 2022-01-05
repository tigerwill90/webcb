package command

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/awnumar/memguard"
	"github.com/docker/go-units"
	"github.com/tigerwill90/webcb/client"
	"github.com/tigerwill90/webcb/client/copyopt"
	grpctls "github.com/tigerwill90/webcb/internal/tls"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"net"
	"os"
	"os/signal"
	"strconv"
	"time"
)

type copyCmd struct {
}

func newCopyCommand() *copyCmd {
	return &copyCmd{}
}

const defaultClientConnTimeout = 5 * time.Second

func (s *copyCmd) run() cli.ActionFunc {
	return func(cc *cli.Context) error {
		defer memguard.Purge()
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

		copyTimeout := cc.Duration(timeout)
		var copyCtx context.Context
		var copyCancel context.CancelFunc
		if copyTimeout == 0 {
			copyCtx, copyCancel = context.WithCancel(context.Background())
		} else {
			copyCtx, copyCancel = context.WithTimeout(context.Background(), copyTimeout)
		}
		defer copyCancel()

		sig := make(chan os.Signal, 2)
		copyErr := make(chan error)
		signal.Notify(sig, os.Interrupt, os.Kill)

		c := client.New(conn)
		go func() {
			copyErr <- c.Copy(
				copyCtx,
				bufio.NewReader(os.Stdin),
				copyopt.WithTransferRate(chunkSize),
				copyopt.WithChecksum(cc.Bool(checksum)),
				copyopt.WithTtl(cc.Duration(ttl)),
				copyopt.WithCompression(cc.Bool(compress)),
				copyopt.WithPassword(cc.String(password)),
			)
		}()

		select {
		case <-sig:
			return fmt.Errorf("copy canceled")
		case cErr := <-copyErr:
			if cErr != nil {
				if errors.Is(copyCtx.Err(), context.DeadlineExceeded) {
					return fmt.Errorf("copy failed: %w", copyCtx.Err())
				}
				return fmt.Errorf("copy failed: %w", cErr)
			}
		}

		return nil
	}
}
