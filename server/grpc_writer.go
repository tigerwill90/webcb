package server

import (
	"github.com/tigerwill90/webcb/proto"
)

type GrpcWriter struct {
	srv    proto.WebClipboard_PasteServer
	t      []byte
	tIndex int
	pIndex int
	tLen   int
	pLen   int
}

func NewGrpcWriter(srv proto.WebClipboard_PasteServer, buf []byte) *GrpcWriter {
	return &GrpcWriter{
		srv:  srv,
		t:    buf,
		tLen: len(buf),
	}
}

func (w *GrpcWriter) Write(p []byte) (int, error) {
	w.pIndex = 0
	w.pLen = len(p)
	for {
		// where len(p) >= len(t)
		if w.pLen > 0 {
			n := copy(w.t[w.tIndex:], p[w.pIndex:])
			w.tIndex += n
			w.pIndex += n
			w.pLen -= n
			w.tLen -= n
			if w.tLen == 0 {
				if err := w.srv.Send(&proto.Stream{Data: &proto.Stream_Chunk{
					Chunk: w.t,
				}}); err != nil {
					return 0, err
				}
				w.tIndex = 0
				w.tLen = len(w.t)
			}
			continue
		}
		break
	}
	return w.pIndex, nil
}

func (w *GrpcWriter) LastWrite() error {
	if w.tLen > 0 {
		return w.srv.Send(&proto.Stream{Data: &proto.Stream_Chunk{
			Chunk: w.t[:w.tIndex],
		}})
	}
	return nil
}

func (w *GrpcWriter) Sum(p []byte) error {
	return w.srv.Send(&proto.Stream{Data: &proto.Stream_Hash{
		Hash: p,
	}})
}
