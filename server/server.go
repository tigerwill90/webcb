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

func NewServer(addr *net.TCPAddr, db *storage.BadgerDB, opts ...Option) *Server {
	config := defaultOption()
	for _, opt := range opts {
		opt.apply(config)
	}

	srv := grpc.NewServer(grpc.MaxRecvMsgSize(config.grpcMaxRecvSize))
	proto.RegisterWebClipboardServer(srv, &webClipboardService{
		db:              db,
		grpcMaxRecvSize: config.grpcMaxRecvSize,
	})
	return &Server{
		tcpAddr: addr,
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
