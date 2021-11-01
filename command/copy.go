package command

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/awnumar/memguard"
	"github.com/docker/go-units"
	"github.com/tigerwill90/webcb/client"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
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
		tcpAddr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(cc.String("host"), strconv.FormatUint(cc.Uint64("port"), 10)))
		if err != nil {
			return err
		}

		chunkSize, err := units.FromHumanSize(cc.String("size"))
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

		timeout := cc.Duration("timeout")
		var copyCtx context.Context
		var copyCancel context.CancelFunc
		if timeout == 0 {
			copyCtx, copyCancel = context.WithCancel(context.Background())
		} else {
			copyCtx, copyCancel = context.WithTimeout(context.Background(), timeout)
		}
		defer copyCancel()

		sig := make(chan os.Signal, 2)
		copyErr := make(chan error)
		signal.Notify(sig, os.Interrupt, os.Kill)

		c := client.New(
			conn,
			client.WithChunkSize(chunkSize),
			client.WithChecksum(cc.Bool("checksum")),
			client.WithTtl(cc.Duration("ttl")),
			client.WithCompression(cc.Bool("compress")),
			client.WithPassword(cc.String("password")),
		)
		go func() {
			copyErr <- c.Copy(copyCtx, bufio.NewReader(os.Stdin))
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
