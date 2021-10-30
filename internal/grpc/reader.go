package grpc

import (
	"io"
)

type Receiver interface {
	Next() ([]byte, error)
	Checksum() []byte
}

func NewReader(recv Receiver) *Reader {
	return &Reader{r: recv}
}

type Reader struct {
	r      Receiver
	t      []byte
	tLen   int
	tIndex int
	pIndex int
}

func (r *Reader) Checksum() []byte {
	return r.r.Checksum()
}

// Read consume a grpc stream and completely fill p (len(p) == n)
// except for the last read where n < p is possible.
func (r *Reader) Read(p []byte) (int, error) {
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

		stdin, err := r.r.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return 0, err
		}

		r.t = stdin
		r.tIndex = 0
		r.tLen = len(stdin)
	}
	// return the number of bytes read in t
	// because the buffer may be filled with
	// old value from pIndex to len(p)
	return r.pIndex, io.EOF
}
