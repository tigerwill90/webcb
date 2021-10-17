package client

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/tigerwill90/webcb/proto"
	"google.golang.org/grpc"
	"io"
	"time"
)

type Client struct {
	c         proto.WebClipboardClient
	chunkSize int64
	integrity bool
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
		integrity: config.integrity,
		ttl:       config.ttl,
	}
}

func (c *Client) Copy(ctx context.Context, r io.Reader) error {
	stream, err := c.c.Copy(ctx)
	if err != nil {
		return err
	}
	rawUuid, err := uuid.New().MarshalBinary()
	if err != nil {
		return err
	}

	if err := sendInfo(stream, rawUuid, c.ttl); err != nil {
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
				if c.integrity {
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
		if c.integrity {
			if _, err := hasher.Write(buf[:n]); err != nil {
				return err
			}
		}
	}

	fmt.Println("bytes sent", send)

	if c.integrity {
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

func sendInfo(stream proto.WebClipboard_CopyClient, uuid []byte, ttl time.Duration) error {
	return stream.Send(&proto.Payload{Data: &proto.Payload_Info_{
		Info: &proto.Payload_Info{
			Uuid: uuid,
			Ttl:  int64(ttl),
		},
	}})
}

func sendChunk(stream proto.WebClipboard_CopyClient, chunk []byte) error {
	return stream.Send(&proto.Payload{Data: &proto.Payload_Chunk{
		Chunk: chunk,
	}})
}

func sendHash(stream proto.WebClipboard_CopyClient, sum []byte) error {
	return stream.Send(&proto.Payload{Data: &proto.Payload_Hash{
		Hash: sum,
	}})
}
