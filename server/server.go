package server

import (
	"context"
	"github.com/tigerwill90/webcb/proto"
	"github.com/tigerwill90/webcb/storage"
	"google.golang.org/grpc"
	"net"
)

type Server struct {
	tcpAddr *net.TCPAddr
	srv     *grpc.Server
}

type Config struct {
	TcpAddr         *net.TCPAddr
	Db              *storage.BadgerDB
	GrpcMaxRecvSize int
}

func NewServer(config Config) *Server {
	srv := grpc.NewServer(grpc.MaxRecvMsgSize(config.GrpcMaxRecvSize))
	proto.RegisterWebClipboardServer(srv, &webClipboardService{
		db: config.Db,
	})
	return &Server{
		tcpAddr: config.TcpAddr,
		srv:     srv,
	}
}

func (s *Server) Start() error {
	lis, err := net.ListenTCP("tcp", s.tcpAddr)
	if err != nil {
		return err
	}
	return s.srv.Serve(lis)
}

func (s *Server) Stop(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.srv.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		s.srv.Stop()
		return ctx.Err()
	}
	return nil
}
