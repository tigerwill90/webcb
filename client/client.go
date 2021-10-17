package client

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/google/uuid"
	"github.com/tigerwill90/webcb/proto"
	"google.golang.org/grpc"
	"io"
)

type Client struct {
	c         proto.WebClipboardClient
	chunkSize int64
	integrity bool
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

	if err := sendInfo(stream, rawUuid, c.chunkSize); err != nil {
		return err
	}

	hasher := sha256.New()
	defer hasher.Reset()

	buf := make([]byte, c.chunkSize)
	send := 0
	for {
		n, err := r.Read(buf)
		send += n
		if err != nil {
			if err != io.EOF {
				return err
			}
			if n > 0 {
				if err := sendChunk(stream, buf[:n]); err != nil {
					return err
				}
				if c.integrity {
					if _, err := hasher.Write(buf[:n]); err != nil {
						return err
					}
				}
			}
			break
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

func sendInfo(stream proto.WebClipboard_CopyClient, uuid []byte, chunkSize int64) error {
	return stream.Send(&proto.Payload{Data: &proto.Payload_Info_{
		Info: &proto.Payload_Info{
			Uuid:      uuid,
			ChunkSize: chunkSize,
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
