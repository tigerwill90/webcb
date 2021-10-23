package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/tigerwill90/webcb/proto"
	"google.golang.org/grpc"
	"io"
	"time"
)

type Client struct {
	c         proto.WebClipboardClient
	chunkSize int64
	checksum  bool
	ttl       time.Duration
}

func New(conn grpc.ClientConnInterface, opts ...Option) *Client {
	config := defaultConfig()
	for _, opt := range opts {
		opt(config)
	}

	return &Client{
		c:         proto.NewWebClipboardClient(conn),
		chunkSize: config.chunkSize,
		checksum:  config.checksum,
		ttl:       config.ttl,
	}
}

func (c *Client) Copy(ctx context.Context, r io.Reader) error {
	stream, err := c.c.Copy(ctx)
	if err != nil {
		return err
	}

	if err := sendInfo(stream, c.ttl); err != nil {
		return err
	}

	hasher := sha256.New()
	defer hasher.Reset()

	fmt.Println("chunk size", c.chunkSize)
	buf := make([]byte, c.chunkSize)
	send := 0
	for {
		n, err := io.ReadAtLeast(r, buf, int(c.chunkSize))
		send += n
		if err != nil {
			// The error is EOF only if no bytes were read
			if errors.Is(err, io.EOF) {
				break
			}
			// If an EOF happens after reading fewer than min bytes,
			// ReadAtLeast returns ErrUnexpectedEOF
			if errors.Is(err, io.ErrUnexpectedEOF) {
				if err := sendChunk(stream, buf[:n]); err != nil {
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
		if err := sendChunk(stream, buf[:n]); err != nil {
			return err
		}
		if c.checksum {
			if _, err := hasher.Write(buf[:n]); err != nil {
				return err
			}
		}
	}

	fmt.Println("bytes sent", send)

	if c.checksum {
		if err := sendHash(stream, hasher.Sum(nil)); err != nil {
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
	hasher := sha256.New()
	defer hasher.Reset()
	received := 0
	var hashIntegrity []byte
	for {
		stream, err := resp.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		switch stream.Data.(type) {
		case *proto.Stream_Info_:
			return errors.New("protocol error: chunk stream expected but get info header")
		case *proto.Stream_Chunk:
			chunk := stream.GetChunk()
			if c.checksum {
				if _, err := hasher.Write(chunk); err != nil {
					return err
				}
			}
			if _, err := w.Write(chunk); err != nil {
				return err
			}
			received += len(chunk)
		case *proto.Stream_Hash:
			hashIntegrity = stream.GetHash()
		}
	}
	if c.checksum {
		if len(hashIntegrity) != sha256.Size {
			return errors.New("invalid sha256 size")
		}
		if n := bytes.Compare(hashIntegrity, hasher.Sum(nil)); n != 0 {
			return errors.New("checksum failed")
		}
	}

	fmt.Println("rcv", received)
	return nil
}

func sendInfo(stream proto.WebClipboard_CopyClient, ttl time.Duration) error {
	return stream.Send(&proto.Stream{Data: &proto.Stream_Info_{
		Info: &proto.Stream_Info{
			Ttl: int64(ttl),
		},
	}})
}

func sendChunk(stream proto.WebClipboard_CopyClient, chunk []byte) error {
	return stream.Send(&proto.Stream{Data: &proto.Stream_Chunk{
		Chunk: chunk,
	}})
}

func sendHash(stream proto.WebClipboard_CopyClient, sum []byte) error {
	return stream.Send(&proto.Stream{Data: &proto.Stream_Hash{
		Hash: sum,
	}})
}
