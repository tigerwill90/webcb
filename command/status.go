package command

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/docker/go-units"
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

type statusCmd struct{}

func newStatusCommand() *statusCmd {
	return &statusCmd{}
}

func (s *statusCmd) run() cli.ActionFunc {
	return func(cc *cli.Context) error {
		tcpAddr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(cc.String(host), strconv.FormatUint(cc.Uint64(port), 10)))
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

		c := client.New(conn)

		serverStatus, err := c.Status(context.Background())
		if err != nil {
			return err
		}
		fmt.Printf("db size: %s\n", units.HumanSize(float64(serverStatus.DbSize)))
		return nil
	}
}
