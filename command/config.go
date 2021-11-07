package command

import (
	"context"
	"fmt"
	"github.com/docker/go-units"
	"github.com/tigerwill90/webcb/client"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
	"net"
	"strconv"
)

type configCmd struct{}

func newConfigCommand() *configCmd {
	return &configCmd{}
}

func (s *configCmd) run() cli.ActionFunc {
	return func(cc *cli.Context) error {
		tcpAddr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(cc.String("host"), strconv.FormatUint(cc.Uint64("port"), 10)))
		if err != nil {
			return err
		}

		connTimeout := cc.Duration("conn-timeout")
		if connTimeout == 0 {
			connTimeout = defaultClientConnTimeout
		}

		ctx, cancel := context.WithTimeout(context.Background(), connTimeout)
		defer cancel()
		conn, err := grpc.DialContext(ctx, tcpAddr.String(), grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			return err
		}

		c := client.New(conn)

		serverConfig, err := c.Config(context.Background())
		if err != nil {
			return err
		}
		fmt.Printf("db size: %s\n", units.HumanSize(float64(serverConfig.DbSize)))
		return nil
	}
}
