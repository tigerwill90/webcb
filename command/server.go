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
	"time"
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

		db, err := storage.NewBadgerDB(&storage.BadgerConfig{
			InMemory:   false,
			GcInterval: 5 * time.Minute,
			Path:       "store",
		}, hclog.New(hclog.DefaultOptions).Named("db"))
		if err != nil {
			return err
		}
		defer db.Close()

		srv := server.NewServer(server.Config{TcpAddr: tcpAddr, Db: db})
		sig := make(chan os.Signal, 2)
		srvErr := make(chan error)
		signal.Notify(sig, os.Interrupt, os.Kill)

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
