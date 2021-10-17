package storage

import (
	"errors"
	"fmt"
	"github.com/dgraph-io/badger/v3"
	"github.com/hashicorp/go-hclog"
	"github.com/oklog/ulid/v2"
	"io"
	"math/rand"
	"strings"
	"sync"
	"time"
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

func (b *BadgerDB) ReadBatch(w io.Writer) (int, error) {
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
				metadata := strings.SplitN(string(k), "/", 3)
				if len(metadata) != 3 {
					return fmt.Errorf("invalid key for index %s", index)
				}
				if metadata[2] == "chunk" {
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
		return nil
	})
	if err != nil {
		return 0, err
	}
	return read, nil
}

type BatchReader interface {
	io.Reader
	Sum() []byte
}

func (b *BadgerDB) WriteBatch(ttl time.Duration, r BatchReader) (int, error) {
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
				e := badger.NewEntry([]byte(key), buf[:nr])
				e.ExpiresAt = uint64(now.Add(ttl).Unix())
				if err := batch.SetEntry(e); err != nil {
					return 0, err
				}
				written += nr
			}
			break
		}
		key := fmt.Sprintf("%d/%s/chunk", version, b.mustNewULID())
		e := badger.NewEntry([]byte(key), buf[:nr])
		e.ExpiresAt = uint64(now.Add(ttl).Unix())
		if err := batch.SetEntry(e); err != nil {
			return 0, err
		}
		written += nr
	}

	sum := r.Sum()
	if len(sum) > 0 {
		key := fmt.Sprintf("%d/%s/hash", version, b.mustNewULID())
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
		valueSize:  64 * 1024,
	}, nil
}
