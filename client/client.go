package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"github.com/klauspost/compress/zstd"
	"github.com/secure-io/sio-go"
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

type Summary struct {
	BytesRead    int
	ByteWrite    int
	CopyDuration time.Duration
	Checksum     []byte
	Encrypted    bool
	Compressed   bool
	TransferRate float64
}

func (c *Client) Copy(ctx context.Context, r io.Reader, opts ...copyopt.Option) (*Summary, error) {
	now := time.Now()
	config := copyopt.DefaultConfig()
	for _, opt := range opts {
		opt.Apply(config)
	}

	stream, err := c.c.Copy(ctx)
	if err != nil {
		return nil, err
	}

	hasher := sha256.New()
	defer hasher.Reset()

	gw := grpchelper.NewWriter(newGrpcSender(stream), make([]byte, config.ChunkSize))
	cnt := BytesWriteCounter(gw)
	w := io.Writer(cnt)
	var enc *zstd.Encoder
	var tw io.WriteCloser

	var masterKeyNonce []byte
	var keyNonce []byte
	var iv []byte
	if len(config.Password) > 0 {
		masterKeyNonce = make([]byte, crypto.NonceSize)
		rand.Read(masterKeyNonce)

		masterKey, err := crypto.DerivePassword(config.Password, masterKeyNonce)
		if err != nil {
			return nil, err
		}

		keyNonce = make([]byte, crypto.NonceSize)
		rand.Read(keyNonce)

		key, err := crypto.DeriveMasterKey(masterKey, keyNonce)
		if err != nil {
			return nil, err
		}

		s, err := sio.AES_256_GCM.Stream(key)
		if err != nil {
			return nil, err
		}

		iv = make([]byte, s.NonceSize())
		rand.Read(iv)

		tw = s.EncryptWriter(w, iv, nil)
		w = tw
	}

	if config.Compression {
		enc, err = zstd.NewWriter(w, zstd.WithEncoderLevel(zstd.SpeedFastest))
		if err != nil {
			return nil, err
		}
		w = enc
	}

	if err := sendInfo(stream, config.Ttl, config.Compression, masterKeyNonce, keyNonce, iv); err != nil {
		return nil, err
	}

	bytesRead := 0
	buf := make([]byte, config.ChunkSize)
	for {
		nr, err := io.ReadAtLeast(r, buf, int(config.ChunkSize))
		bytesRead += nr
		if err != nil {
			// The error is EOF only if no bytes were read
			if errors.Is(err, io.EOF) {
				if enc != nil {
					if err := enc.Close(); err != nil {
						return nil, err
					}
				}
				if tw != nil {
					if err := tw.Close(); err != nil {
						return nil, err
					}
				}

				break
			}
			// If an EOF happens after reading fewer than min bytes,
			// ReadAtLeast returns ErrUnexpectedEOF
			if errors.Is(err, io.ErrUnexpectedEOF) {
				if _, err := w.Write(buf[:nr]); err != nil {
					return nil, err
				}

				if config.Checksum {
					if _, err := hasher.Write(buf[:nr]); err != nil {
						return nil, err
					}
				}
				continue
			}

			return nil, err
		}

		if _, err := w.Write(buf[:nr]); err != nil {
			return nil, err
		}
		if config.Checksum {
			if _, err := hasher.Write(buf[:nr]); err != nil {
				return nil, err
			}
		}
	}

	var checksum []byte
	if config.Checksum {
		checksum = hasher.Sum(nil)
	}
	if err := gw.Flush(checksum); err != nil {
		return nil, err
	}

	_, err = stream.CloseAndRecv()
	if err != nil {
		return nil, err
	}

	d := time.Since(now)
	return &Summary{
		BytesRead:    bytesRead,
		ByteWrite:    cnt.Sent(),
		CopyDuration: d,
		Checksum:     checksum,
		Encrypted:    len(config.Password) > 0,
		Compressed:   config.Compression,
		TransferRate: float64(cnt.Sent()) / d.Seconds(),
	}, nil
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

	if len(config.Password) == 0 && (len(info.MasterKeyNonce) != 0 || len(info.KeyNonce) != 0) {
		return errors.New("data is encrypted but no password provided")
	}

	gr := grpchelper.NewReader(newGrpcReceiver(resp))
	r := io.Reader(gr)
	if len(config.Password) > 0 && len(info.MasterKeyNonce) == crypto.NonceSize && len(info.KeyNonce) == crypto.NonceSize {
		masterKey, err := crypto.DerivePassword(config.Password, info.MasterKeyNonce)
		if err != nil {
			return err
		}

		key, err := crypto.DeriveMasterKey(masterKey, info.KeyNonce)
		if err != nil {
			return err
		}

		s, err := sio.AES_256_GCM.Stream(key)
		if err != nil {
			return err
		}

		r = s.DecryptReader(r, info.Iv, nil)
	}

	if info.Compressed {
		dec, err := zstd.NewReader(r)
		if err != nil {
			return err
		}
		defer dec.Close()
		r = dec
	}

	buf := make([]byte, config.ChunkSize)
	for {
		n, err := r.Read(buf)
		if err != nil {
			if !errors.Is(err, io.EOF) {
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
	DbSize              float64
	DbPath              string
	GrpcMaxReceiveBytes float64
	GcInterval          time.Duration
	DevMOde             bool
	GrpcMTLS            bool
}

func (c *Client) Status(ctx context.Context) (*ServerConfig, error) {
	config, err := c.c.Status(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	return &ServerConfig{
		DbSize:              float64(config.DbSize),
		DbPath:              config.DbPath,
		GrpcMaxReceiveBytes: float64(config.GrpcMaxReceiveBytes),
		GcInterval:          time.Duration(config.GcInterval),
		DevMOde:             config.DevMode,
		GrpcMTLS:            config.GrpcMTls,
	}, nil
}

func sendInfo(stream proto.WebClipboard_CopyClient, ttl time.Duration, compressed bool, masterKeyNonce []byte, keyNonce []byte, iv []byte) error {
	return stream.Send(&proto.CopyStream{Data: &proto.CopyStream_Info_{
		Info: &proto.CopyStream_Info{
			Ttl:            int64(ttl),
			Compressed:     compressed,
			MasterKeyNonce: masterKeyNonce,
			KeyNonce:       keyNonce,
			Iv:             iv,
		},
	}})
}

type grpcSender struct {
	c         proto.WebClipboard_CopyClient
	bytesSent int
}

func newGrpcSender(client proto.WebClipboard_CopyClient) *grpcSender {
	return &grpcSender{c: client}
}

func (gc *grpcSender) SendChunk(p []byte) error {
	gc.bytesSent += len(p)
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

type Counter struct {
	w        io.Writer
	r        io.Reader
	sent     int
	received int
}

func BytesWriteCounter(w io.Writer) *Counter {
	return &Counter{w: w}
}

func BytesReadCounter(r io.Reader) *Counter {
	return &Counter{r: r}
}

func (cnt *Counter) Read(p []byte) (n int, err error) {
	n, err = cnt.r.Read(p)
	cnt.received += n
	return
}

func (cnt *Counter) Write(p []byte) (n int, err error) {
	n, err = cnt.w.Write(p)
	cnt.sent += n
	return
}

func (cnt *Counter) Received() int {
	return cnt.received
}

func (cnt *Counter) Sent() int {
	return cnt.sent
}
