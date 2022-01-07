package server

import (
	"context"
	"crypto/tls"
	"errors"
	"github.com/hashicorp/go-hclog"
	grpctls "github.com/tigerwill90/webcb/internal/tls"
	"github.com/tigerwill90/webcb/proto"
	"github.com/tigerwill90/webcb/storage"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"net"
)

type Server struct {
	tcpAddr *net.TCPAddr
	srv     *grpc.Server
}

func NewServer(addr *net.TCPAddr, db *storage.BadgerDB, logger hclog.Logger, opts ...Option) (*Server, error) {
	config := defaultOption()
	for _, opt := range opts {
		opt.apply(config)
	}

	options := []grpc.ServerOption{grpc.MaxRecvMsgSize(config.grpcMaxRecvSize)}
	if !config.dev {
		if len(config.cert) == 0 || len(config.key) == 0 {
			return nil, errors.New("tls certificate is required in non dev mode environment")
		}
		tlsConfig := &tls.Config{
			ClientAuth: tls.RequireAndVerifyClientCert,
		}
		if err := grpctls.LoadCertificate(config.ca, config.cert, config.key, tlsConfig); err != nil {
			return nil, err
		}
		options = append(options, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}

	srv := grpc.NewServer(options...)
	proto.RegisterWebClipboardServer(srv, &webClipboardService{
		db:     db,
		config: config,
		logger: logger,
	})

	return &Server{
		tcpAddr: addr,
		srv:     srv,
	}, nil
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
