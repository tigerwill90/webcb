package command

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/go-hclog"
	"github.com/tigerwill90/webcb/server"
	"github.com/tigerwill90/webcb/storage"
	"github.com/urfave/cli/v2"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

type serverCmd struct {
}

func newServerCmd() *serverCmd {
	return &serverCmd{}
}

func (s *serverCmd) run() cli.ActionFunc {
	return func(cc *cli.Context) error {
		host := cc.String(hostFlag)
		if host == "" {
			host = defaultServerAddr
		}

		tcpAddr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(host, strconv.FormatUint(cc.Uint64(portFlag), 10)))
		if err != nil {
			return err
		}

		dbGcInterval := cc.Duration(gcIntervalFlag)
		if dbGcInterval < storage.MinGcInterval {
			dbGcInterval = storage.MinGcInterval
		}

		logOpts := hclog.DefaultOptions
		logOpts.Color = hclog.AutoColor
		srvLogger := hclog.New(logOpts).Named("server")
		srvLogger.SetLevel(hclog.Trace)

		var inMemory bool
		if cc.Bool(devModeFlag) && cc.String(pathFlag) == "" {
			inMemory = true
		}

		if !cc.Bool(devModeFlag) && cc.String(pathFlag) == "" {
			return errors.New("a path is required to persist clipboard data when dev mode is disable")
		}

		path := cc.String(filepath.Clean(pathFlag))

		if cc.String(path) != "" {
			if err := os.MkdirAll(path, 0700); err != nil {
				return err
			}
		}

		dbLogger := srvLogger.Named("db")
		db, err := storage.NewBadgerDB(&storage.BadgerConfig{
			InMemory:   inMemory,
			GcInterval: dbGcInterval,
			Path:       path,
		}, dbLogger)
		if err != nil {
			return err
		}
		defer db.Close()

		var ca, cert, key []byte
		if !cc.Bool(devModeFlag) {
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
		}

		srv, err := server.NewServer(tcpAddr, db, srvLogger, server.WithGrpcMaxRecvSize(cc.Int(grpcMaxReceivedBytesFlag)), server.WithCredentials(cert, key), server.WithCertificateAuthority(ca), server.WithDevMode(cc.Bool(devModeFlag)))
		if err != nil {
			return err
		}
		sig := make(chan os.Signal, 1)
		srvErr := make(chan error)
		signal.Notify(sig, os.Interrupt, os.Kill, syscall.SIGTERM)

		go func() {
			log.Printf("server started on port %d\n", tcpAddr.Port)
			srvErr <- srv.Start()
		}()

		select {
		case <-sig:
		case err := <-srvErr:
			if err != nil {
				log.Println(err)
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		fmt.Println("server shutdown", srv.Stop(ctx))

		return nil
	}
}
