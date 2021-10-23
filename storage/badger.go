package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/ristretto/z"
	"github.com/hashicorp/go-hclog"
	"github.com/oklog/ulid/v2"
	"io"
	"math/rand"
	"sync"
	"time"
)

const (
	ChunkSize = 64 * 1024
)

type BadgerConfig struct {
	Path       string
	InMemory   bool
	GcInterval time.Duration
	ValueSize  int
}

type BadgerDB struct {
	db         *badger.DB
	gcInterval *time.Ticker
	logger     hclog.Logger
	config     *BadgerConfig
	seq        *badger.Sequence
	entropy    *ulid.MonotonicEntropy
	t          time.Time
	valueSize  int
	sync.Mutex
}

var ErrKeyNotFound = errors.New("key not found")

func (b *BadgerDB) ReadStream() error {
	var prefix []byte
	if err := b.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("latest"))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrKeyNotFound
			}
			return err
		}

		prefix, err = item.ValueCopy(nil)
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	stream := b.db.NewStream()

	stream.NumGo = 16
	stream.Prefix = prefix
	stream.LogPrefix = "badger.streaming"

	stream.ChooseKey = func(item *badger.Item) bool {
		return bytes.HasSuffix(item.Key(), []byte("chunk"))
	}

	stream.KeyToList = stream.ToList

	stream.Send = func(buf *z.Buffer) error {
		kvList, err := badger.BufferToKVList(buf)
		if err != nil {
			return err
		}
		for _, kv := range kvList.GetKv() {
			fmt.Println(string(kv.GetKey()))
		}
		return nil
	}

	return stream.Orchestrate(context.Background())
}

type StreamWriter interface {
	io.Writer
	LastWrite() error
	Sum(b []byte) error
}

func (b *BadgerDB) ReadBatch(w StreamWriter) (int, error) {
	read := 0
	err := b.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("latest"))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrKeyNotFound
			}
			return err
		}
		index, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte(fmt.Sprintf("%s/", index))
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			k := item.Key()
			err := item.Value(func(v []byte) error {
				if bytes.HasSuffix(k, []byte("chunk")) {
					valCopy := append(v[:0:0], v...)
					nw, err := w.Write(valCopy)
					if err != nil {
						return err
					}
					read += nw
				}
				return nil
			})
			if err != nil {
				return err
			}
		}

		if err := w.LastWrite(); err != nil {
			return err
		}

		hashKey := fmt.Sprintf("%s/sha256/hash", index)
		item, err = txn.Get([]byte(hashKey))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				fmt.Println("no hash found")
				return nil
			}
			return err
		}

		return item.Value(func(v []byte) error {
			valCopy := append(v[:0:0], v...)
			return w.Sum(valCopy)
		})
	})
	if err != nil {
		return 0, err
	}
	return read, nil
}

type StreamReader interface {
	io.Reader
	Sum() []byte
}

// TODO maybe use io.ReadAtLeast ?
func (b *BadgerDB) WriteBatch(ttl time.Duration, r StreamReader) (int, error) {
	buf := make([]byte, b.valueSize)
	version, err := b.seq.Next()
	if err != nil {
		return 0, err
	}
	batch := b.db.NewWriteBatch()
	defer batch.Cancel()
	written := 0
	now := time.Now()
	for {
		nr, err := r.Read(buf)
		if err != nil {
			if err != io.EOF {
				return 0, err
			}
			if nr > 0 {
				key := fmt.Sprintf("%d/%s/chunk", version, b.mustNewULID())
				valCopy := append(buf[:0:0], buf[:nr]...)
				e := badger.NewEntry([]byte(key), valCopy)
				e.ExpiresAt = uint64(now.Add(ttl).Unix())
				if err := batch.SetEntry(e); err != nil {
					return 0, err
				}
				written += nr
			}
			break
		}
		key := fmt.Sprintf("%d/%s/chunk", version, b.mustNewULID())
		valCopy := append(buf[:0:0], buf[:nr]...)
		e := badger.NewEntry([]byte(key), valCopy)
		e.ExpiresAt = uint64(now.Add(ttl).Unix())
		if err := batch.SetEntry(e); err != nil {
			return 0, err
		}
		written += nr
	}

	sum := r.Sum()
	if len(sum) > 0 {
		key := fmt.Sprintf("%d/sha256/hash", version)
		e := badger.NewEntry([]byte(key), sum)
		e.ExpiresAt = uint64(now.Add(ttl).Unix())
		if err := batch.SetEntry(e); err != nil {
			return 0, err
		}
	}

	key := fmt.Sprintf("%d", version)
	e := badger.NewEntry([]byte("latest"), []byte(key))
	e.ExpiresAt = uint64(now.Add(ttl).Unix())
	if err := batch.SetEntry(e); err != nil {
		return 0, err
	}

	if err := batch.Flush(); err != nil {
		return 0, err
	}

	return written, nil
}

func (b *BadgerDB) Close() error {
	b.gcInterval.Stop()
	if err := b.seq.Release(); err != nil {
		b.logger.Log(hclog.Error, err.Error())
	}
	return b.db.Close()
}

func (b *BadgerDB) mustNewULID() ulid.ULID {
	b.Lock()
	defer b.Unlock()
	return ulid.MustNew(ulid.Timestamp(b.t), b.entropy)
}

func NewBadgerDB(config *BadgerConfig, logger hclog.Logger) (*BadgerDB, error) {
	opt := badger.DefaultOptions(config.Path).
		WithInMemory(config.InMemory).
		WithLoggingLevel(badger.ERROR).
		WithNumVersionsToKeep(0)

	db, err := badger.Open(opt)
	if err != nil {
		return nil, err
	}

	seq, err := db.GetSequence([]byte("version"), 1000)
	if err != nil {
		return nil, err
	}

	gcInterval := time.NewTicker(config.GcInterval)
	go func() {
		for range gcInterval.C {
		again:
			err := db.RunValueLogGC(0.1)
			if err == nil {
				logger.Trace("GC run successfully")
				goto again
			}
			logger.Trace(err.Error())
		}
	}()

	t := time.Now()
	return &BadgerDB{
		db:         db,
		seq:        seq,
		gcInterval: gcInterval,
		logger:     logger,
		config:     config,
		entropy:    ulid.Monotonic(rand.New(rand.NewSource(t.Unix())), 0),
		t:          t,
		valueSize:  ChunkSize,
	}, nil
}
