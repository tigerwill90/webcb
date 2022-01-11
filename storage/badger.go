package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/dgraph-io/badger/v2"
	"github.com/hashicorp/go-hclog"
	"github.com/oklog/ulid/v2"
	"github.com/tigerwill90/webcb/proto"
	protobuf "google.golang.org/protobuf/proto"
	"io"
	"math/rand"
	"sync"
	"time"
)

const (
	chunkSize           = 64 * 1024
	latestKey           = "LATEST"
	versionKey          = "VERSION"
	pendingTxnKey       = "PENDING"
	chunkSuffix         = "chunk"
	discardKeyMaxNumber = 20_000
	discardKeyTimeout   = 100 * time.Millisecond
	MinGcInterval       = 1 * time.Minute
	ValueLogFileSize    = 500 << 20
)

type BadgerConfig struct {
	Path       string
	InMemory   bool
	GcInterval time.Duration
	ValueSize  int
}

type BadgerDB struct {
	db                *badger.DB
	gcInterval        *time.Ticker
	logger            hclog.Logger
	config            *BadgerConfig
	seq               *badger.Sequence
	entropy           *ulid.MonotonicEntropy
	t                 time.Time
	valueLogFileSize  int
	chunkSize         int
	lockedULID        sync.Mutex
	lockedTransaction sync.Mutex
	sync.RWMutex
}

var ErrKeyNotFound = errors.New("key not found")

func (b *BadgerDB) DropAll() error {
	b.Lock()
	defer b.Unlock()
	b.logger.Warn("dropping clipboard data")
	return b.db.DropAll()
}

type StreamWriter interface {
	io.Writer
	Flush(checksum []byte) error
}

type HeaderWriter interface {
	Write(compressed, hasChecksum bool, masterKeyNonce, keyNonce []byte) error
}

func (b *BadgerDB) ReadBatch(sw StreamWriter, hw HeaderWriter) (int, error) {
	b.RLock()
	defer b.RUnlock()
	b.logger.Trace("sending clipboard stream to client")
	read := 0
	err := b.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(latestKey))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrKeyNotFound
			}
			return err
		}

		value, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}

		cb := new(proto.Clipboard)
		if err := protobuf.Unmarshal(value, cb); err != nil {
			return err
		}

		if err := hw.Write(cb.Compressed, len(cb.Checksum) > 0, cb.MasterKeyNonce, cb.KeyNonce); err != nil {
			return err
		}

		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte(fmt.Sprintf("%d/", cb.Sequence))
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			k := item.Key()
			err := item.Value(func(v []byte) error {
				if bytes.HasSuffix(k, []byte(chunkSuffix)) {
					valCopy := append(v[:0:0], v...)
					nw, err := sw.Write(valCopy)
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

		return sw.Flush(cb.Checksum)
	})
	if err != nil {
		return 0, err
	}
	return read, nil
}

type StreamReader interface {
	io.Reader
	Checksum() []byte
}

func (b *BadgerDB) WriteBatch(r StreamReader, ttl time.Duration, compressed bool, masterKeyNonce []byte, keyNonce []byte) (int, error) {
	b.RLock()
	defer b.RUnlock()
	b.logger.Trace("receiving clipboard stream from client")
	buf := make([]byte, b.chunkSize)
	version, err := b.seq.Next()
	if err != nil {
		return 0, err
	}
	batch := b.NewBatchWriter(version)
	defer func() {
		if err := batch.Release(); err != nil {
			b.logger.Error(fmt.Sprintf("unable to unlock transaction: %s", err))
		}
	}()
	written := 0
	for {
		nr, err := r.Read(buf)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return 0, err
			}
			if nr > 0 {
				key := []byte(fmt.Sprintf("%d/%s/%s", version, b.mustNewULID(), chunkSuffix))
				valCopy := append(buf[:0:0], buf[:nr]...)
				if err := batch.Validate(key, valCopy); err != nil {
					return 0, err
				}
				e := badger.NewEntry(key, valCopy)
				if err := batch.SetEntry(e); err != nil {
					return 0, err
				}
				written += nr
			}
			break
		}
		key := []byte(fmt.Sprintf("%d/%s/%s", version, b.mustNewULID(), chunkSuffix))
		valCopy := append(buf[:0:0], buf[:nr]...)
		if err := batch.Validate(key, valCopy); err != nil {
			return 0, err
		}
		e := badger.NewEntry(key, valCopy)
		if err := batch.SetEntry(e); err != nil {
			return 0, err
		}
		written += nr
	}

	cb := &proto.Clipboard{
		Sequence:       version,
		ExpireAt:       time.Now().Add(ttl).Format(time.RFC1123Z),
		Compressed:     compressed,
		MasterKeyNonce: masterKeyNonce,
		KeyNonce:       keyNonce,
		Checksum:       r.Checksum(),
		Size:           int64(written),
	}

	value, err := protobuf.Marshal(cb)
	if err != nil {
		return 0, err
	}
	key := []byte(latestKey)
	if err := batch.Validate(key, value); err != nil {
		return 0, err
	}
	e := badger.NewEntry(key, value).WithTTL(ttl)
	if err := batch.SetEntry(e); err != nil {
		return 0, err
	}

	if err := batch.Flush(); err != nil {
		return 0, err
	}

	return written, nil
}

func (b *BadgerDB) Size() (lsm int64, vlog int64) {
	b.RLock()
	defer b.RUnlock()
	return b.db.Size()
}

func (b *BadgerDB) Path() string {
	return b.config.Path
}

func (b *BadgerDB) InMemory() bool {
	return b.config.InMemory
}

func (b *BadgerDB) GcInterval() time.Duration {
	return b.config.GcInterval
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
	path := config.Path
	if config.InMemory {
		logger.Trace("starting database in memory mode")
		path = ""
	}
	opts := badger.DefaultOptions(path).
		WithInMemory(config.InMemory).
		WithLoggingLevel(badger.ERROR).
		WithNumVersionsToKeep(1).
		WithNumLevelZeroTables(1).
		WithValueLogFileSize(ValueLogFileSize)

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
		db:               db,
		gcInterval:       gcInterval,
		logger:           logger,
		config:           config,
		seq:              seq,
		entropy:          ulid.Monotonic(rand.New(rand.NewSource(t.Unix())), 0),
		t:                t,
		valueLogFileSize: ValueLogFileSize,
		chunkSize:        chunkSize,
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
				ctx, cancel := context.WithTimeout(context.Background(), discardKeyTimeout)
				defer cancel()
				deleted, err := b.discard(ctx, discardKeyMaxNumber)
				if err != nil {
					b.logger.Error(err.Error())
					return
				}
				b.logger.Trace(fmt.Sprintf("GC discarded %d keys", deleted))
			}()
		}
	}()

	return b, nil
}
