package client

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/sha256"
	"errors"
	"github.com/klauspost/compress/zstd"
	"github.com/tigerwill90/webcb/client/copyopt"
	"github.com/tigerwill90/webcb/client/pasteopt"
	"github.com/tigerwill90/webcb/internal/crypto"
	grpchelper "github.com/tigerwill90/webcb/internal/grpc"
	"github.com/tigerwill90/webcb/proto"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"io"
	"math/rand"
	"time"
)

type Client struct {
	c proto.WebClipboardClient
}

func New(conn grpc.ClientConnInterface) *Client {
	return &Client{
		c: proto.NewWebClipboardClient(conn),
	}
}

func (c *Client) Copy(ctx context.Context, r io.Reader, opts ...copyopt.Option) error {
	config := copyopt.DefaultConfig()
	for _, opt := range opts {
		opt.Apply(config)
	}

	stream, err := c.c.Copy(ctx)
	if err != nil {
		return err
	}

	hasher := sha256.New()
	defer hasher.Reset()

	gw := grpchelper.NewWriter(newGrpcSender(stream), make([]byte, config.ChunkSize))
	w := io.Writer(gw)
	var enc *zstd.Encoder
	if config.Compression {
		enc, err = zstd.NewWriter(w, zstd.WithEncoderLevel(zstd.SpeedFastest))
		if err != nil {
			return err
		}
		w = enc
		defer enc.Close()
	}

	var salt []byte
	var iv []byte
	if len(config.Password) > 0 {
		randSalt := make([]byte, crypto.SaltSize)
		rand.Read(randSalt)
		randIv := make([]byte, aes.BlockSize)
		rand.Read(randIv)

		salt = randSalt
		iv = randIv

		key, err := crypto.DeriveKey(config.Password, salt)
		if err != nil {
			return err
		}

		sw, err := crypto.NewStreamWriter(key, iv, w)
		if err != nil {
			return err
		}
		defer sw.Close()
		w = sw
	}

	if err := sendInfo(stream, config.Ttl, config.Compression, iv, salt); err != nil {
		return err
	}

	buf := make([]byte, config.ChunkSize)
	for {
		n, err := io.ReadAtLeast(r, buf, int(config.ChunkSize))
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
				if enc != nil {
					if err := enc.Close(); err != nil {
						return err
					}
				}
				if config.Checksum {
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
		if config.Checksum {
			if _, err := hasher.Write(buf[:n]); err != nil {
				return err
			}
		}
	}

	var checksum []byte
	if config.Checksum {
		checksum = hasher.Sum(nil)
	}
	if err := gw.Flush(checksum); err != nil {
		return err
	}

	_, err = stream.CloseAndRecv()
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) Paste(ctx context.Context, w io.Writer, opts ...pasteopt.Option) error {
	config := pasteopt.DefaultConfig()
	for _, opt := range opts {
		opt.Apply(config)
	}

	resp, err := c.c.Paste(ctx, &proto.PasteOption{TransferRate: config.ChunkSize}, grpc.MaxCallRecvMsgSize(int(config.ChunkSize+1000)))
	if err != nil {
		return err
	}

	hasher := sha256.New()
	defer hasher.Reset()

	stream, err := resp.Recv()
	if err != nil {
		return err
	}

	var info *proto.PastStream_Info
	var pasteErr *proto.PastStream_Error
	switch stream.Data.(type) {
	case *proto.PastStream_Info_:
		info = stream.GetInfo()
	case *proto.PastStream_Error_:
		pasteErr = stream.GetError()
	default:
		return errors.New("protocol error: info or error message expected but get a chunk message")
	}

	if pasteErr != nil {
		return errors.New(pasteErr.Message)
	}

	if len(config.Password) == 0 && (len(info.Iv) != 0 || len(info.Salt) != 0) {
		return errors.New("data is encrypted but no password provided")
	}

	gr := grpchelper.NewReader(newGrpcReceiver(resp))
	r := io.Reader(gr)
	if info.Compressed {
		dec, err := zstd.NewReader(r)
		if err != nil {
			return err
		}
		defer dec.Close()
		r = dec
	}
	if len(config.Password) > 0 && len(info.Iv) == aes.BlockSize && len(info.Salt) == crypto.SaltSize {
		key, err := crypto.DeriveKey(config.Password, info.Salt)
		if err != nil {
			return err
		}

		sr, err := crypto.NewStreamReader(key, info.Iv, r)
		if err != nil {
			return err
		}
		r = sr
	}

	buf := make([]byte, config.ChunkSize)
	for {
		n, err := r.Read(buf)
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
				return err
			}

			if n > 0 {
				if info.Checksum {
					if _, err := hasher.Write(buf[:n]); err != nil {
						return err
					}
				}
				if _, err := w.Write(buf[:n]); err != nil {
					return err
				}
			}

			break
		}

		if info.Checksum {
			if _, err := hasher.Write(buf[:n]); err != nil {
				return err
			}
		}
		if _, err := w.Write(buf[:n]); err != nil {
			return err
		}

	}

	if info.Checksum {
		if len(gr.Checksum()) != sha256.Size {
			return errors.New("invalid sha256 size")
		}
		if n := bytes.Compare(gr.Checksum(), hasher.Sum(nil)); n != 0 {
			return errors.New("checksum failed")
		}
	}

	return nil
}

func (c *Client) Clean(ctx context.Context) error {
	_, err := c.c.Clean(ctx, &emptypb.Empty{})
	return err
}

type ServerConfig struct {
	DbSize int64
}

func (c *Client) Status(ctx context.Context) (*ServerConfig, error) {
	config, err := c.c.Status(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	return &ServerConfig{
		DbSize: config.DbSize,
	}, nil
}

func sendInfo(stream proto.WebClipboard_CopyClient, ttl time.Duration, compressed bool, iv []byte, salt []byte) error {
	return stream.Send(&proto.CopyStream{Data: &proto.CopyStream_Info_{
		Info: &proto.CopyStream_Info{
			Ttl:        int64(ttl),
			Compressed: compressed,
			Salt:       salt,
			Iv:         iv,
		},
	}})
}

type grpcSender struct {
	c proto.WebClipboard_CopyClient
}

func newGrpcSender(client proto.WebClipboard_CopyClient) *grpcSender {
	return &grpcSender{client}
}

func (gc *grpcSender) SendChunk(p []byte) error {
	return gc.c.Send(&proto.CopyStream{Data: &proto.CopyStream_Chunk{
		Chunk: p,
	}})
}

func (gc *grpcSender) SendChecksum(p []byte) error {
	if len(p) == 0 {
		return nil
	}
	return gc.c.Send(&proto.CopyStream{Data: &proto.CopyStream_Checksum{
		Checksum: p,
	}})
}

func newGrpcReceiver(client proto.WebClipboard_PasteClient) *grpcReceiver {
	return &grpcReceiver{c: client}
}

type grpcReceiver struct {
	c        proto.WebClipboard_PasteClient
	checksum []byte
}

func (gc *grpcReceiver) Next() ([]byte, error) {
	stream, err := gc.c.Recv()
	if err != nil {
		return nil, err
	}

	switch stream.Data.(type) {
	case *proto.PastStream_Info_:
		return nil, errors.New("protocol error: chunk message expected but get an info message")
	case *proto.PastStream_Error_:
		return nil, errors.New("protocol error: chunk message expected but get an error message")
	case *proto.PastStream_Checksum:
		gc.checksum = stream.GetChecksum()
		return nil, io.EOF
	default:
	}
	return stream.GetChunk(), nil
}

func (gc *grpcReceiver) Checksum() []byte {
	return gc.checksum
}
