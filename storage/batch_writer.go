package storage

import (
	"errors"
	"github.com/dgraph-io/badger/v2"
	"github.com/tigerwill90/webcb/proto"
	protobuf "google.golang.org/protobuf/proto"
	"hash/crc32"
	"strconv"
	"sync"
)

const maxHeaderSize = 21

type BatchWriter struct {
	*badger.WriteBatch
	b         *BadgerDB
	batchSize int
	version   uint64
	once      sync.Once
}

func (b *BadgerDB) NewBatchWriter(version uint64) *BatchWriter {
	return &BatchWriter{
		WriteBatch: b.db.NewWriteBatch(),
		b:          b,
		version:    version,
	}
}

func (bw *BatchWriter) lockVersion() error {
	bw.b.lockedTransaction.Lock()
	defer bw.b.lockedTransaction.Unlock()
	var err error
	bw.once.Do(func() {
		err = bw.b.db.Update(func(txn *badger.Txn) error {
			lockedTxn := new(proto.LockedTransaction)
			item, err := txn.Get([]byte(pendingTxnKey))
			if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}

			if err == nil {
				if err := item.Value(func(val []byte) error {
					return protobuf.Unmarshal(val, lockedTxn)
				}); err != nil {
					return err
				}
			}

			if len(lockedTxn.Versions) == 0 {
				lockedTxn.Versions = make(map[string][]byte, 1)
			}

			lockedTxn.Versions[strconv.FormatUint(bw.version, 10)] = nil

			value, err := protobuf.Marshal(lockedTxn)
			if err != nil {
				return err
			}
			return txn.Set([]byte(pendingTxnKey), value)
		})
	})
	return nil
}

func (bw *BatchWriter) unlockVersion() error {
	bw.b.lockedTransaction.Lock()
	defer bw.b.lockedTransaction.Unlock()
	return bw.b.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(pendingTxnKey))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return nil
			}
			return err
		}
		lockedTxn := new(proto.LockedTransaction)
		if err := item.Value(func(val []byte) error {
			return protobuf.Unmarshal(val, lockedTxn)
		}); err != nil {
			return err
		}

		delete(lockedTxn.Versions, strconv.FormatUint(bw.version, 10))

		value, err := protobuf.Marshal(lockedTxn)
		if err != nil {
			return err
		}
		return txn.Set([]byte(pendingTxnKey), value)
	})
}

func (bw *BatchWriter) Validate(key, value []byte) error {
	bw.batchSize += estimateEntrySize(key, value)
	if bw.batchSize > bw.b.valueLogFileSize {
		bw.b.logger.Warn("committing transaction: batch size overflow")
		if err := bw.lockVersion(); err != nil {
			return err
		}
		if err := bw.Flush(); err != nil {
			return err
		}
		bw.WriteBatch = bw.b.db.NewWriteBatch()
		bw.batchSize = 0
	}
	return nil
}

func (bw *BatchWriter) Release() error {
	bw.WriteBatch.Cancel()
	return bw.unlockVersion()
}

func estimateEntrySize(key, value []byte) int {
	return maxHeaderSize + len(key) + len(value) + crc32.Size
}
