package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"io"
)

type StreamWriter struct {
	sw *cipher.StreamWriter
}

func NewStreamWriter(key []byte, iv []byte, w io.Writer) (*StreamWriter, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	stream := cipher.NewOFB(block, iv)
	return &StreamWriter{
		sw: &cipher.StreamWriter{S: stream, W: w},
	}, nil
}

func (w *StreamWriter) Write(p []byte) (int, error) {
	return w.sw.Write(p)
}

func (w *StreamWriter) Close() error {
	return w.sw.Close()
}

type StreamReader struct {
	sr *cipher.StreamReader
}

func NewStreamReader(key []byte, iv []byte, r io.Reader) (*StreamReader, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	stream := cipher.NewOFB(block, iv)
	return &StreamReader{
		sr: &cipher.StreamReader{S: stream, R: r},
	}, nil
}

func (r *StreamReader) Read(p []byte) (int, error) {
	return r.sr.Read(p)
}
