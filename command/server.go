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
		tcpAddr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(cc.String(host), strconv.FormatUint(cc.Uint64(port), 10)))
		if err != nil {
			return err
		}

		dbGcInterval := cc.Duration(gcInterval)
		if dbGcInterval < storage.MinGcInterval {
			dbGcInterval = storage.MinGcInterval
		}

		srvLogger := hclog.New(hclog.DefaultOptions).Named("server")
		srvLogger.SetLevel(hclog.Trace)

		var inMemory bool
		if cc.Bool(devMode) && cc.String(dbPath) == "" {
			inMemory = true
		}

		if !cc.Bool(devMode) && cc.String(dbPath) == "" {
			return errors.New("a path is required to persist clipboard data when dev mode is disable")
		}

		path := cc.String(filepath.Clean(dbPath))

		if cc.String(path) != "" {
			if err := os.MkdirAll(path, 0700); err != nil {
				return err
			}
		}

		dbLogger := srvLogger.Named("db")
		db, err := storage.NewBadgerDB(&storage.BadgerConfig{
			InMemory:   inMemory,
			GcInterval: dbGcInterval,
			Path:       cc.String(dbPath),
		}, dbLogger)
		if err != nil {
			return err
		}
		defer db.Close()

		var ca, cert, key []byte
		if !cc.Bool(devMode) {
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
		}

		srv, err := server.NewServer(tcpAddr, db, srvLogger, server.WithGrpcMaxRecvSize(cc.Int(grpcMaxReceivedBytes)), server.WithCredentials(cert, key), server.WithCertificateAuthority(ca), server.WithDevMode(cc.Bool(devMode)))
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
