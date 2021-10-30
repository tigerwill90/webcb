package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"github.com/klauspost/compress/zstd"
	grpchelper "github.com/tigerwill90/webcb/internal/grpc"
	"github.com/tigerwill90/webcb/proto"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"io"
	"time"
)

type Client struct {
	c           proto.WebClipboardClient
	chunkSize   int64
	checksum    bool
	ttl         time.Duration
	compression bool
}

func New(conn grpc.ClientConnInterface, opts ...Option) *Client {
	config := defaultConfig()
	for _, opt := range opts {
		opt(config)
	}

	return &Client{
		c:           proto.NewWebClipboardClient(conn),
		chunkSize:   config.chunkSize,
		checksum:    config.checksum,
		ttl:         config.ttl,
		compression: config.compression,
	}
}

func (c *Client) Copy(ctx context.Context, r io.Reader) error {
	stream, err := c.c.Copy(ctx)
	if err != nil {
		return err
	}

	if err := sendInfo(stream, c.ttl, c.compression); err != nil {
		return err
	}

	hasher := sha256.New()
	defer hasher.Reset()

	gw := grpchelper.NewWriter(newGrpcCopy(stream), make([]byte, c.chunkSize))
	w := io.Writer(gw)
	if c.compression {
		enc, err := zstd.NewWriter(w)
		if err != nil {
			return err
		}
		defer enc.Close()
		w = enc
	}

	buf := make([]byte, c.chunkSize)
	for {
		n, err := io.ReadAtLeast(r, buf, int(c.chunkSize))
		if err != nil {
			// The error is EOF only if no bytes were read
			if errors.Is(err, io.EOF) {
				break
			}
			// If an EOF happens after reading fewer than min bytes,
			// ReadAtLeast returns ErrUnexpectedEOF
			if errors.Is(err, io.ErrUnexpectedEOF) {
				if _, err := w.Write(buf[:n]); err != nil {
					return err
				}
				if c.checksum {
					if _, err := hasher.Write(buf[:n]); err != nil {
						return err
					}
				}
				continue
			}

			return err
		}

		if _, err := w.Write(buf[:n]); err != nil {
			return err
		}
		if c.checksum {
			if _, err := hasher.Write(buf[:n]); err != nil {
				return err
			}
		}
	}

	if err := gw.Flush(); err != nil {
		return err
	}

	if c.checksum {
		if err := gw.Checksum(hasher.Sum(nil)); err != nil {
			return err
		}
	}

	_, err = stream.CloseAndRecv()
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) Paste(ctx context.Context, w io.Writer) error {
	resp, err := c.c.Paste(ctx, &proto.PasteOption{Length: c.chunkSize}, grpc.MaxCallRecvMsgSize(int(c.chunkSize+1000)))
	if err != nil {
		return err
	}

	var (
		hasChecksum        bool
		checksum           []byte
		chunkStreamStarted bool
	)

	hasher := sha256.New()
	defer hasher.Reset()

	for {
		stream, err := resp.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		switch stream.Data.(type) {
		case *proto.PastStream_Info_:
			if chunkStreamStarted {
				return errors.New("protocol error: chunk stream expected but get info header")
			}
			hasChecksum = true
			checksum = stream.GetInfo().GetChecksum()
		case *proto.PastStream_Chunk:
			chunkStreamStarted = true
			chunk := stream.GetChunk()
			if hasChecksum {
				if _, err := hasher.Write(chunk); err != nil {
					return err
				}
			}
			if _, err := w.Write(chunk); err != nil {
				return err
			}
		}
	}
	if hasChecksum {
		if len(checksum) != sha256.Size {
			return errors.New("invalid sha256 size")
		}
		if n := bytes.Compare(checksum, hasher.Sum(nil)); n != 0 {
			return errors.New("checksum failed")
		}
	}

	return nil
}

func (c *Client) Clean(ctx context.Context) error {
	_, err := c.c.Clean(ctx, &emptypb.Empty{})
	return err
}

func sendInfo(stream proto.WebClipboard_CopyClient, ttl time.Duration, compressed bool) error {
	return stream.Send(&proto.CopyStream{Data: &proto.CopyStream_Info_{
		Info: &proto.CopyStream_Info{
			Ttl:        int64(ttl),
			Compressed: compressed,
		},
	}})
}

type grpcCopy struct {
	c proto.WebClipboard_CopyClient
}

func newGrpcCopy(client proto.WebClipboard_CopyClient) *grpcCopy {
	return &grpcCopy{client}
}

func (gc *grpcCopy) SendChunk(p []byte) error {
	return gc.c.Send(&proto.CopyStream{Data: &proto.CopyStream_Chunk{
		Chunk: p,
	}})
}

func (gc *grpcCopy) SendChecksum(p []byte) error {
	return gc.c.Send(&proto.CopyStream{Data: &proto.CopyStream_Checksum{
		Checksum: p,
	}})
}
