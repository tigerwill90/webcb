package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/tigerwill90/webcb/internal/grpc"
	"github.com/tigerwill90/webcb/proto"
	"github.com/tigerwill90/webcb/storage"
	"google.golang.org/protobuf/types/known/emptypb"
	"io"
	"time"
)

const (
	DefaultTtl       = 10 * time.Minute
	DefaultWriteSize = 1 * 1024 * 1024
)

type webClipboardService struct {
	proto.UnsafeWebClipboardServer
	db *storage.BadgerDB
}

func (s *webClipboardService) Clean(ctx context.Context, empty *emptypb.Empty) (*emptypb.Empty, error) {
	if err := s.db.DropAll(); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *webClipboardService) Copy(server proto.WebClipboard_CopyServer) error {
	req, err := server.Recv()
	if err != nil {
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
	n, err := s.db.WriteBatch(r, ttl, fi.Compressed)
	if err != nil {
		return err
	}

	fmt.Println("bytes written to db:", n)
	fmt.Printf("hash %x\n", r.Checksum())
	return server.SendAndClose(&emptypb.Empty{})
}

func (s *webClipboardService) Paste(option *proto.PasteOption, server proto.WebClipboard_PasteServer) error {
	if option.Length == 0 {
		option.Length = DefaultWriteSize
	}
	fmt.Println("chunk size", option.Length)
	sender := newGrpcSender(server)
	w := grpc.NewWriter(sender, make([]byte, option.Length))
	nr, err := s.db.ReadBatch(w, sender)
	if err != nil {
		return err
	}

	fmt.Println("bytes read from db:", nr)
	return nil
}

type grpcSender struct {
	srv proto.WebClipboard_PasteServer
}

func newGrpcSender(srv proto.WebClipboard_PasteServer) *grpcSender {
	return &grpcSender{srv}
}

func (gp *grpcSender) Write(compressed, hasChecksum bool) error {
	return gp.srv.Send(&proto.PastStream{Data: &proto.PastStream_Info_{
		Info: &proto.PastStream_Info{
			Checksum:   hasChecksum,
			Compressed: compressed,
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
