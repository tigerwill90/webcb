package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/go-hclog"
	"github.com/tigerwill90/webcb/internal/grpc"
	"github.com/tigerwill90/webcb/proto"
	"github.com/tigerwill90/webcb/storage"
	"google.golang.org/protobuf/types/known/emptypb"
	"io"
	"time"
)

const (
	DefaultTtl          = 10 * time.Minute
	DefaultTransferRate = 1 << 20
)

type ErrorType int32

const (
	InvalidOption = iota
	KeyNotFound
)

type webClipboardService struct {
	proto.UnsafeWebClipboardServer
	db     *storage.BadgerDB
	config *config
	logger hclog.Logger
}

func (s *webClipboardService) Status(ctx context.Context, _ *emptypb.Empty) (*proto.ServerStatus, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	lsm, vlog := s.db.Size()
	return &proto.ServerStatus{
		DbSize:              lsm + vlog,
		DbPath:              s.db.Path(),
		GrpcMaxReceiveBytes: int64(s.config.grpcMaxRecvSize),
		GcInterval:          int64(s.db.GcInterval()),
		DevMode:             s.config.dev,
		GrpcMTls:            len(s.config.key) > 0 && len(s.config.cert) > 0,
	}, nil
}

func (s *webClipboardService) Clean(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if err := s.db.DropAll(); err != nil {
		s.logger.Error(err.Error())
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *webClipboardService) Copy(server proto.WebClipboard_CopyServer) error {
	req, err := server.Recv()
	if err != nil {
		s.logger.Error(err.Error())
		return err
	}

	fi := req.GetInfo()
	if fi == nil {
		return errors.New("protocol error: expected an info header but get a chunk stream")
	}

	ttl := time.Duration(fi.Ttl)
	if ttl == 0 {
		ttl = DefaultTtl
	}

	r := grpc.NewReader(newGrpcReceiver(server))
	_, err = s.db.WriteBatch(r, ttl, fi.Compressed, fi.MasterKeyNonce, fi.KeyNonce)
	if err != nil {
		s.logger.Error(err.Error())
		return err
	}

	return server.SendAndClose(&emptypb.Empty{})
}

func (s *webClipboardService) Paste(option *proto.PasteOption, server proto.WebClipboard_PasteServer) error {
	if option.TransferRate == 0 {
		option.TransferRate = DefaultTransferRate
	}

	sender := newGrpcSender(server)

	if option.TransferRate > int64(s.config.grpcMaxRecvSize) {
		return sender.SendError(errors.New("transfer rate is above the server configuration limit"), InvalidOption)
	}

	w := grpc.NewWriter(sender, make([]byte, option.TransferRate))
	_, err := s.db.ReadBatch(w, sender)
	if err != nil {
		if errors.Is(err, storage.ErrKeyNotFound) {
			return sender.SendError(fmt.Errorf("no data to paste: %w", err), KeyNotFound)
		}
		s.logger.Error(err.Error())
		return err
	}

	return nil
}

type grpcSender struct {
	srv proto.WebClipboard_PasteServer
}

func newGrpcSender(srv proto.WebClipboard_PasteServer) *grpcSender {
	return &grpcSender{srv}
}

func (gp *grpcSender) SendError(err error, kind ErrorType) error {
	return gp.srv.Send(&proto.PastStream{Data: &proto.PastStream_Error_{
		Error: &proto.PastStream_Error{
			Type:    proto.PastStream_Error_Type(kind),
			Message: err.Error(),
		},
	}})
}

func (gp *grpcSender) Write(compressed, hasChecksum bool, masterKeyNonce, keyNonce []byte) error {
	return gp.srv.Send(&proto.PastStream{Data: &proto.PastStream_Info_{
		Info: &proto.PastStream_Info{
			Checksum:       hasChecksum,
			Compressed:     compressed,
			MasterKeyNonce: masterKeyNonce,
			KeyNonce:       keyNonce,
		},
	}})
}

func (gp *grpcSender) SendChunk(p []byte) error {
	return gp.srv.Send(&proto.PastStream{Data: &proto.PastStream_Chunk{
		Chunk: p,
	}})
}

func (gp *grpcSender) SendChecksum(p []byte) error {
	if len(p) == 0 {
		return nil
	}
	return gp.srv.Send(&proto.PastStream{Data: &proto.PastStream_Checksum{
		Checksum: p,
	}})
}

func newGrpcReceiver(srv proto.WebClipboard_CopyServer) *grpcReceiver {
	return &grpcReceiver{srv: srv}
}

type grpcReceiver struct {
	srv      proto.WebClipboard_CopyServer
	checksum []byte
}

func (gc *grpcReceiver) Next() ([]byte, error) {
	stream, err := gc.srv.Recv()
	if err != nil {
		return nil, err
	}

	switch stream.Data.(type) {
	case *proto.CopyStream_Info_:
		return nil, errors.New("protocol error: chunk stream expected but get info header")
	case *proto.CopyStream_Checksum:
		gc.checksum = stream.GetChecksum()
		return nil, io.EOF
	default:
	}
	return stream.GetChunk(), nil
}

func (gc *grpcReceiver) Checksum() []byte {
	return gc.checksum
}
