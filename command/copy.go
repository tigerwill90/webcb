package command

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
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
	ui *ui
}

func newCopyCommand(ui *ui) *copyCmd {
	return &copyCmd{
		ui: ui,
	}
}

const defaultClientConnTimeout = 5 * time.Second

type result struct {
	summary *client.Summary
	err     error
}

func (cmd *copyCmd) run() cli.ActionFunc {
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

		/*		if err := clipboard.Init(); err != nil {
				return err
			}*/

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

		copyTimeout := cc.Duration(timeoutFlag)
		var copyCtx context.Context
		var copyCancel context.CancelFunc
		if copyTimeout == 0 {
			copyCtx, copyCancel = context.WithCancel(context.Background())
		} else {
			copyCtx, copyCancel = context.WithTimeout(context.Background(), copyTimeout)
		}
		defer copyCancel()

		sig := make(chan os.Signal, 2)
		resultStream := make(chan result)
		signal.Notify(sig, os.Interrupt, os.Kill)

		c := client.New(conn)
		go func() {
			summary, err := c.Copy(
				copyCtx,
				bufio.NewReader(os.Stdin),
				client.Password(readSecret),
				copyopt.WithTransferRate(chunkSize),
				copyopt.WithChecksum(cc.Bool(checksumFlag)),
				copyopt.WithTtl(cc.Duration(ttlFlag)),
				copyopt.WithCompression(cc.Bool(compressFlag)),
				copyopt.WithEncryption(!cc.Bool(noPasswordFlag)),
			)
			resultStream <- result{summary: summary, err: err}
		}()

		select {
		case <-sig:
			return fmt.Errorf("copy canceled")
		case res := <-resultStream:
			if res.err != nil {
				if errors.Is(copyCtx.Err(), context.DeadlineExceeded) {
					return fmt.Errorf("copy failed: %w", copyCtx.Err())
				}
				return fmt.Errorf("copy failed: %w", res.err)
			}

			if cc.Bool(verboseFlag) {
				cmd.ui.Successf("Successfully copied data to web clipboard!\n\n")
				cmd.ui.Infof("Duration     : %s\n", formatDuration(res.summary.CopyDuration))
				cmd.ui.Infof("Read         : %s\n", units.HumanSize(float64(res.summary.BytesRead)))
				cmd.ui.Infof("Sent         : %s\n", units.HumanSize(float64(res.summary.ByteWrite)))
				if len(res.summary.Checksum) > 0 {
					cmd.ui.Infof("Digest       : %x\n", res.summary.Checksum)
				}
				cmd.ui.Infof("Compressed   : %t\n", res.summary.Compressed)
				cmd.ui.Infof("Encrypted    : %t\n", res.summary.Encrypted)
				cmd.ui.Infof("Upload speed : %s/s\n", units.HumanSize(res.summary.TransferRate))
			}
		}

		return nil
	}
}
