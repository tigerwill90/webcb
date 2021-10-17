package server

import (
	"errors"
	"fmt"
	"github.com/tigerwill90/webcb/proto"
	"github.com/tigerwill90/webcb/storage"
	"google.golang.org/protobuf/types/known/emptypb"
	"io"
	"time"
)

const DefaultChunkSize int64 = 32 * 1024

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

	copyR := newCopyReader(server)
	copyW, err := s.db.NewBatchWriter(time.Hour)
	if err != nil {
		return err
	}
	defer func() {
		fmt.Println("commit err", copyW.Commit())
	}()

	n, err := copyChunk(copyW, copyR, DefaultChunkSize)
	if err != nil {
		return err
	}
	if len(copyR.sum()) > 0 {
		if err := copyW.SetHash(copyR.sum()); err != nil {

			return err
		}
	}

	fmt.Printf("hash %x\n", copyR.sum())
	fmt.Println("bytes written: ", n)
	fmt.Println("Close send!!!!!")
	return server.SendAndClose(&emptypb.Empty{})
}

func (s *webClipboardService) Paste(_ *emptypb.Empty, server proto.WebClipboard_PasteServer) error {
	panic("todo implement me")
}

func newCopyReader(srv proto.WebClipboard_CopyServer) *copyReader {
	return &copyReader{srv: srv}
}

type copyReader struct {
	srv    proto.WebClipboard_CopyServer
	t      []byte
	tLen   int
	tIndex int
	pIndex int
	hash   []byte
}

func (r *copyReader) sum() []byte {
	return r.hash
}

// Read consume a grpc stream and completely fill p (len(p) == n)
// except for the last read where n <= p is possible.
func (r *copyReader) Read(p []byte) (n int, err error) {
	r.pIndex = 0
	for {
		if r.tLen > 0 {
			n = copy(p[r.pIndex:], r.t[r.tIndex:])
			r.tIndex += n
			r.tLen -= n
			if n+r.pIndex == len(p) {
				return n + r.pIndex, nil
			}
			r.pIndex = n + r.pIndex
		}

		stream, err := r.srv.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return 0, err
		}

		switch stream.Data.(type) {
		case *proto.Payload_Info_:
			return 0, errors.New("protocol error: chunk stream expected but get info header")
		case *proto.Payload_Hash:
			r.hash = stream.GetHash()
			return r.pIndex, io.EOF
		default:
		}

		stdin := stream.GetChunk()
		r.t = stdin
		r.tIndex = 0
		r.tLen = len(stdin)
	}
	// return the number of bytes read in t
	// because the buffer may be filled with
	// old value from pIndex to len(p)
	return r.pIndex, io.EOF
}

func copyChunk(dest io.Writer, src io.Reader, chunkSize int64) (int, error) {
	buf := make([]byte, chunkSize)
	written := 0
	for {
		nr, err := src.Read(buf)
		if err != nil {
			if err != io.EOF {
				return 0, err
			}
			if nr > 0 {
				nw, err := dest.Write(buf[:nr])
				if err != nil {
					return 0, err
				}
				written += nw
			}
			break
		}
		nw, err := dest.Write(buf[:nr])
		if err != nil {
			return 0, err
		}
		written += nw
	}
	return written, nil
}
