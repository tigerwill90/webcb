package enclave

import (
	"github.com/awnumar/memguard"
	"sync"
)

type Enclave struct {
	locked *memguard.Enclave
	once   sync.Once
}

func New(buf []byte) *Enclave {
	locked := memguard.NewBufferFromBytes(buf)
	return &Enclave{locked: locked.Seal()}
}

type DestroyFunc func()

func (e *Enclave) Open() ([]byte, DestroyFunc) {
	buf, err := e.locked.Open()
	if err != nil {
		memguard.SafePanic(err)
	}

	destroy := func() {
		e.once.Do(func() { buf.Destroy() })
	}

	return buf.Bytes(), destroy
}
