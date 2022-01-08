package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/dgraph-io/badger/v2"
	"github.com/tigerwill90/webcb/proto"
	protobuf "google.golang.org/protobuf/proto"
	"math/rand"
)

func (b *BadgerDB) runGC(discardRatio float64) error {
	b.RLock()
	defer b.RUnlock()
	return b.db.RunValueLogGC(discardRatio)
}

func getIndex(item *badger.Item) ([]byte, error) {
	if item == nil {
		return nil, nil
	}
	cb := new(proto.Clipboard)
	if err := item.Value(func(val []byte) error {
		return protobuf.Unmarshal(val, cb)
	}); err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf("%d/", cb.Sequence)), nil
}

func getLockedTransaction(item *badger.Item) (map[string][]byte, error) {
	if item == nil {
		return make(map[string][]byte), nil
	}
	lockedTransaction := new(proto.LockedTransaction)
	if err := item.Value(func(val []byte) error {
		return protobuf.Unmarshal(val, lockedTransaction)
	}); err != nil {
		return nil, err
	}
	return lockedTransaction.Versions, nil
}

func (b *BadgerDB) discard(ctx context.Context, keys int) (int, error) {
	b.RLock()
	defer b.RUnlock()
	b.logger.Trace(fmt.Sprintf("attempting to discard %d keys", keys))
	deleted := 0

	txn := b.db.NewTransaction(true)
	defer txn.Discard()

	item, err := txn.Get([]byte(latestKey))
	if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
		return 0, err
	}

	index, err := getIndex(item)
	if err != nil {
		return 0, err
	}

	item, err = txn.Get([]byte(pendingTxnKey))
	if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
		return 0, err
	}

	lockedTransaction, err := getLockedTransaction(item)
	if err != nil {
		return 0, err
	}

	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = false
	opts.Reverse = rand.Intn(2) == 1

	it := txn.NewIterator(opts)
	defer it.Close()

STOP:
	for it.Rewind(); it.Valid(); it.Next() {
		select {
		case <-ctx.Done():
			b.logger.Warn("GC stopped after timeout")
			break STOP
		default:
		}
		if deleted >= keys {
			break
		}
		item := it.Item()
		key := item.Key()

		if (index == nil || !bytes.HasPrefix(key, index)) &&
			!bytes.Equal(key, []byte(latestKey)) &&
			!bytes.Equal(key, []byte(versionKey)) &&
			!bytes.Equal(key, []byte(pendingTxnKey)) {

			keyCopy := item.KeyCopy(nil)
			if splits := bytes.Split(keyCopy, []byte("/")); len(splits) > 0 {
				if _, ok := lockedTransaction[string(splits[0])]; ok {
					continue
				}
			}

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
