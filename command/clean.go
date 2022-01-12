package command

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/tigerwill90/webcb/client"
	grpctls "github.com/tigerwill90/webcb/internal/tls"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"net"
	"os"
	"strconv"
)

type cleanCmd struct{}

func newCleanCommand() *cleanCmd {
	return &cleanCmd{}
}

func (c *cleanCmd) run() cli.ActionFunc {
	return func(cc *cli.Context) error {
		host := cc.String(hostFlag)
		if host == "" {
			host = defaultClientAddr
		}

		address := net.JoinHostPort(host, strconv.FormatUint(cc.Uint64(portFlag), 10))

		connexionTimeout := cc.Duration(connTimeoutFlag)
		if connexionTimeout == 0 {
			connexionTimeout = defaultClientConnTimeout
		}

		var options []grpc.DialOption
		if cc.Bool(connInsecureFlag) {
			options = append(options, grpc.WithTransportCredentials(insecure.NewCredentials()))
		} else {
			var ca, cert, key []byte
			var err error
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

		c := client.New(conn)

		return c.Clean(context.Background())
	}
}
