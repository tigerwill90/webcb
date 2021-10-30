package grpc

type Sender interface {
	SendChunk(p []byte) error
	SendChecksum(p []byte) error
}

type Writer struct {
	s      Sender
	t      []byte
	tIndex int
	pIndex int
	tLen   int
	pLen   int
}

func NewWriter(sender Sender, buf []byte) *Writer {
	return &Writer{
		s:    sender,
		t:    buf,
		tLen: len(buf),
	}
}

func (w *Writer) Write(p []byte) (int, error) {
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
				if err := w.s.SendChunk(w.t); err != nil {
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

func (w *Writer) Flush() error {
	if w.tLen > 0 {
		return w.s.SendChunk(w.t[:w.tIndex])
	}
	return nil
}

func (w *Writer) Checksum(p []byte) error {
	return w.s.SendChecksum(p)
}
