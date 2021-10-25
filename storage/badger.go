package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/dgraph-io/badger/v2"
	"github.com/hashicorp/go-hclog"
	"github.com/oklog/ulid/v2"
	"io"
	"math/rand"
	"sync"
	"time"
)

const (
	chunkSize     = 64 * 1024
	latestKey     = "latest"
	versionKey    = "version"
	chunkSuffix   = "chunk"
	hashSuffix    = "hash"
	MinGcDuration = 1 * time.Minute
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
	lockedULID sync.Mutex
	sync.RWMutex
}

var ErrKeyNotFound = errors.New("key not found")

type StreamWriter interface {
	io.Writer
	LastWrite() error
	Sum(b []byte) error
}

func (b *BadgerDB) ReadBatch(w StreamWriter) (int, error) {
	b.RLock()
	defer b.RUnlock()
	read := 0
	err := b.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(latestKey))
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
				if bytes.HasSuffix(k, []byte(chunkSuffix)) {
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

		hashKey := fmt.Sprintf("%s/sha256/%s", index, hashSuffix)
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

func (b *BadgerDB) WriteBatch(ttl time.Duration, r StreamReader) (int, error) {
	b.RLock()
	defer b.RUnlock()
	buf := make([]byte, b.valueSize)
	version, err := b.seq.Next()
	if err != nil {
		return 0, err
	}
	batch := b.db.NewWriteBatch()
	defer batch.Cancel()
	written := 0
	for {
		nr, err := r.Read(buf)
		if err != nil {
			if err != io.EOF {
				return 0, err
			}
			if nr > 0 {
				key := fmt.Sprintf("%d/%s/%s", version, b.mustNewULID(), chunkSuffix)
				valCopy := append(buf[:0:0], buf[:nr]...)
				e := badger.NewEntry([]byte(key), valCopy)
				if err := batch.SetEntry(e); err != nil {
					return 0, err
				}
				written += nr
			}
			break
		}
		key := fmt.Sprintf("%d/%s/%s", version, b.mustNewULID(), chunkSuffix)
		valCopy := append(buf[:0:0], buf[:nr]...)
		e := badger.NewEntry([]byte(key), valCopy)
		if err := batch.SetEntry(e); err != nil {
			return 0, err
		}
		written += nr
	}

	sum := r.Sum()
	if len(sum) > 0 {
		key := fmt.Sprintf("%d/sha256/%s", version, hashSuffix)
		e := badger.NewEntry([]byte(key), sum)
		if err := batch.SetEntry(e); err != nil {
			return 0, err
		}
	}

	key := fmt.Sprintf("%d", version)
	e := badger.NewEntry([]byte(latestKey), []byte(key)).WithTTL(ttl)
	if err := batch.SetEntry(e); err != nil {
		return 0, err
	}

	if err := batch.Flush(); err != nil {
		return 0, err
	}

	return written, nil
}

func (b *BadgerDB) runGC(discardRatio float64) error {
	b.RLock()
	defer b.RUnlock()
	return b.db.RunValueLogGC(discardRatio)
}

func (b *BadgerDB) discard(ctx context.Context, keys int) (int, error) {
	b.RLock()
	defer b.RUnlock()
	deleted := 0

	txn := b.db.NewTransaction(true)
	defer txn.Discard()

	item, err := txn.Get([]byte(latestKey))
	if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
		return 0, err
	}

	var valCopy []byte
	if item != nil {
		valCopy, err = item.ValueCopy(nil)
		if err != nil {
			return 0, err
		}
	}

	index := []byte(fmt.Sprintf("%s/", valCopy))
	opts := badger.DefaultIteratorOptions
	it := txn.NewIterator(opts)
	defer it.Close()

STOP:
	for it.Rewind(); it.Valid(); it.Next() {
		select {
		case <-ctx.Done():
			break STOP
		default:
		}
		if deleted >= keys {
			break
		}
		item := it.Item()
		key := item.Key()
		if (item == nil || !bytes.HasPrefix(key, index)) && !bytes.Equal(key, []byte(latestKey)) && !bytes.Equal(key, []byte(versionKey)) {
			keyCopy := item.KeyCopy(nil)
			err := txn.Delete(keyCopy)
			if err != nil {
				if errors.Is(err, badger.ErrTxnTooBig) {
					if err := txn.Commit(); err != nil {
						return 0, err
					}
					txn = b.db.NewTransaction(true)
					if err := txn.Delete(keyCopy); err != nil {
						return 0, err
					}
					deleted++
					continue
				}
				return 0, err
			}
			deleted++
		}
	}

	it.Close()
	if err := txn.Commit(); err != nil {
		return 0, err
	}

	return deleted, nil
}

func (b *BadgerDB) mustNewULID() ulid.ULID {
	b.lockedULID.Lock()
	defer b.lockedULID.Unlock()
	return ulid.MustNew(ulid.Timestamp(b.t), b.entropy)
}

func (b *BadgerDB) Close() error {
	b.gcInterval.Stop()
	if err := b.seq.Release(); err != nil {
		b.logger.Log(hclog.Error, err.Error())
	}
	return b.db.Close()
}

func NewBadgerDB(config *BadgerConfig, logger hclog.Logger) (*BadgerDB, error) {
	opts := badger.DefaultOptions(config.Path).
		WithInMemory(config.InMemory).
		WithLoggingLevel(badger.ERROR).
		WithNumVersionsToKeep(1).
		WithNumLevelZeroTables(1).
		WithValueLogFileSize(500 << 20)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	seq, err := db.GetSequence([]byte(versionKey), 1000)
	if err != nil {
		return nil, err
	}

	gcInterval := time.NewTicker(config.GcInterval)

	t := time.Now()
	b := &BadgerDB{
		db:         db,
		seq:        seq,
		gcInterval: gcInterval,
		logger:     logger,
		config:     config,
		entropy:    ulid.Monotonic(rand.New(rand.NewSource(t.Unix())), 0),
		t:          t,
		valueSize:  chunkSize,
	}

	go func() {
		for range gcInterval.C {
			func() {
			AGAIN:
				err := b.runGC(0.5)
				if err == nil {
					b.logger.Trace("GC compaction done")
					goto AGAIN
				}
				b.logger.Trace(err.Error())
				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()
				deleted, err := b.discard(ctx, 10000)
				if err != nil {
					b.logger.Error(err.Error())
					return
				}
				b.logger.Trace(fmt.Sprintf("GC deleted %d keys", deleted))
			}()
		}
	}()

	return b, nil
}
