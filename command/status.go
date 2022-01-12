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

type statusCmd struct {
	ui *ui
}

func newStatusCommand(ui *ui) *statusCmd {
	return &statusCmd{
		ui: ui,
	}
}

func (s *statusCmd) run() cli.ActionFunc {
	return func(cc *cli.Context) error {
		host := cc.String(hostFlag)
		if host == "" {
			host = "127.0.0.1"
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

		serverStatus, err := c.Status(context.Background())
		if err != nil {
			return err
		}

		s.ui.Infof("Webcb server status\n")
		s.ui.Infof("DB Size           : %s\n", units.HumanSize(serverStatus.DbSize))
		s.ui.Infof("DB Path           : %s\n", serverStatus.DbPath)
		s.ui.Infof("Max accepted rate : %s\n", units.HumanSize(serverStatus.GrpcMaxReceiveBytes))
		s.ui.Infof("GC Interval       : %s\n", formatDuration(serverStatus.GcInterval))
		s.ui.Infof("Dev mode enable   : %t\n", serverStatus.DevMOde)
		s.ui.Infof("MTLS enable       : %t\n", serverStatus.GrpcMTLS)
		return nil
	}
}
