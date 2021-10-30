package command

import (
	"context"
	"errors"
	"fmt"
	"github.com/docker/go-units"
	"github.com/gen2brain/beeep"
	"github.com/tigerwill90/webcb/client"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
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
		var pasteCtx context.Context
		var pasteCancel context.CancelFunc
		if timeout == 0 {
			pasteCtx, pasteCancel = context.WithCancel(context.Background())
		} else {
			pasteCtx, pasteCancel = context.WithTimeout(context.Background(), timeout)
		}
		defer pasteCancel()

		var stdout io.Writer = os.Stdout
		if cc.Bool("discard") {
			stdout = io.Discard
		}

		c := client.New(conn, client.WithChunkSize(chunkSize))

		sig := make(chan os.Signal, 2)
		pastErr := make(chan error)
		signal.Notify(sig, os.Interrupt, os.Kill)
		go func() {
			pastErr <- c.Paste(pasteCtx, stdout)
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
