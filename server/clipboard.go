package server

import (
	"errors"
	"fmt"
	"github.com/tigerwill90/webcb/proto"
	"github.com/tigerwill90/webcb/storage"
	"google.golang.org/protobuf/types/known/emptypb"
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

func (s *webClipboardService) Copy(server proto.WebClipboard_CopyServer) error {
	req, err := server.Recv()
	if err != nil {
		return err
	}

	fi := req.GetInfo()
	if fi == nil {
		return errors.New("protocol error: expected an info header but get a chunk stream")
	}

	if fi.Ttl == 0 {
		fi.Ttl = int64(DefaultTtl)
	}

	r := NewGrpcReader(server)
	n, err := s.db.WriteBatch(time.Duration(fi.Ttl), r)
	if err != nil {
		return err
	}

	fmt.Println("bytes written to db:", n)
	fmt.Printf("hash %x\n", r.Sum())
	return server.SendAndClose(&emptypb.Empty{})
}

func (s *webClipboardService) Paste(option *proto.PasteOption, server proto.WebClipboard_PasteServer) error {
	if option.Length == 0 {
		option.Length = DefaultWriteSize
	}
	fmt.Println("chunk size", option.Length)
	w := NewGrpcWriter(server, make([]byte, option.Length))
	nr, err := s.db.ReadBatch(w)
	if err != nil {
		return err
	}

	fmt.Println("bytes read from db:", nr)
	return nil
}
