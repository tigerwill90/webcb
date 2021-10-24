package command

import (
	"context"
	"fmt"
	"github.com/hashicorp/go-hclog"
	"github.com/tigerwill90/webcb/server"
	"github.com/tigerwill90/webcb/storage"
	"github.com/urfave/cli/v2"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

const (
	defaultGrpcMaxRecvBytes = 200 * 1024 * 1024
)

type serverCmd struct {
}

func newServerCmd() *serverCmd {
	return &serverCmd{}
}

func (s *serverCmd) run() cli.ActionFunc {
	return func(cc *cli.Context) error {
		tcpAddr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(cc.String("host"), strconv.FormatUint(cc.Uint64("port"), 10)))
		if err != nil {
			return err
		}

		dbLogger := hclog.New(hclog.DefaultOptions).Named("db")
		dbLogger.SetLevel(hclog.Trace)
		db, err := storage.NewBadgerDB(&storage.BadgerConfig{
			InMemory:   false,
			GcInterval: 1 * time.Minute,
			Path:       "store",
		}, dbLogger)
		if err != nil {
			return err
		}
		defer db.Close()

		grpcMaxRecvBytes := cc.Int("grpc-max-receive-bytes")
		if grpcMaxRecvBytes == 0 {
			grpcMaxRecvBytes = defaultGrpcMaxRecvBytes
		}

		srv := server.NewServer(server.Config{TcpAddr: tcpAddr, Db: db, GrpcMaxRecvSize: grpcMaxRecvBytes})
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
			log.Println(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		fmt.Println("server shutdown", srv.Stop(ctx))

		return nil
	}
}
