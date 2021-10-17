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

const (
	readSize   int64 = 64 * 1024
	DefaultTtl       = 10 * time.Minute
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

	copyR := newCopyReader(server)
	n, err := s.db.WriteBatch(time.Duration(fi.Ttl), copyR)
	if err != nil {
		return err
	}

	nr, err := s.db.ReadBatch(io.Discard)
	if err != nil {
		return err
	}

	fmt.Printf("hash %x\n", copyR.Sum())
	fmt.Println("bytes written: ", n)
	fmt.Println("bytes read:", nr)
	fmt.Println("Close send!!!!!")
	return server.SendAndClose(&emptypb.Empty{})
}

func (s *webClipboardService) Paste(option *proto.PasteOption, server proto.WebClipboard_PasteServer) error {
	panic("todo implement me")
}

func newCopyReader(srv proto.WebClipboard_CopyServer) *copyReader {
	return &copyReader{srv: srv}
}

type pastWriter struct {
	srv    proto.WebClipboard_PasteServer
	t      []byte
	tIndex int
	pIndex int
	tLen   int
	pLen   int
}

/*func (w *pastWriter) Write(p []byte) (int, error) {
	w.pIndex = 0
	for {
		if w.tLen > 0 {
			n := copy(w.t[w.tIndex:], p[w.pIndex:])
			w.tIndex += n
			w.pIndex += n
			w.tLen-=n
			if w.tLen == 0 {
				// send

			}
		}
	}

	w.srv.Send()
}*/

type copyReader struct {
	srv    proto.WebClipboard_CopyServer
	t      []byte
	tLen   int
	tIndex int
	pIndex int
	hash   []byte
}

func (r *copyReader) Sum() []byte {
	return r.hash
}

// Read consume a grpc stream and completely fill p (len(p) == n)
// except for the last read where n < p is possible.
func (r *copyReader) Read(p []byte) (int, error) {
	r.pIndex = 0
	for {
		if r.tLen > 0 {
			n := copy(p[r.pIndex:], r.t[r.tIndex:])
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
