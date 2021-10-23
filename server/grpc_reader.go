package server

import (
	"errors"
	"github.com/tigerwill90/webcb/proto"
	"io"
)

func NewGrpcReader(srv proto.WebClipboard_CopyServer) *GrpcReader {
	return &GrpcReader{srv: srv}
}

type GrpcReader struct {
	srv    proto.WebClipboard_CopyServer
	t      []byte
	tLen   int
	tIndex int
	pIndex int
	hash   []byte
}

func (r *GrpcReader) Sum() []byte {
	return r.hash
}

// Read consume a grpc stream and completely fill p (len(p) == n)
// except for the last read where n < p is possible.
func (r *GrpcReader) Read(p []byte) (int, error) {
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
		case *proto.Stream_Info_:
			return 0, errors.New("protocol error: chunk stream expected but get info header")
		case *proto.Stream_Hash:
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
