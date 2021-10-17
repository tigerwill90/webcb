package storage

import (
	"context"
	"errors"
	"fmt"
	"github.com/dgraph-io/badger/v3"
	"github.com/hashicorp/go-hclog"
	"time"
)

type BadgerConfig struct {
	Path       string
	InMemory   bool
	GcInterval time.Duration
}

type BadgerDB struct {
	db         *badger.DB
	gcInterval *time.Ticker
	logger     hclog.Logger
	config     *BadgerConfig
	seq        *badger.Sequence
}

type Entry struct {
	Key   []byte
	Value []byte
}

var (
	ErrNoRecords   = errors.New("no record found")
	ErrRecordExist = errors.New("record already exist")
)

func (b *BadgerDB) Put(ctx context.Context, entry *Entry) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	err := b.db.Update(func(txn *badger.Txn) error {
		_, err := txn.Get(entry.Key)
		if err != nil && errors.Is(err, badger.ErrKeyNotFound) {
			return txn.Set(entry.Key, entry.Value)
		}
		if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		return fmt.Errorf("a record exist for %s key: %w", entry.Key, ErrRecordExist)
	})
	return err
}

func (b *BadgerDB) Update(ctx context.Context, entry *Entry) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	err := b.db.Update(func(txn *badger.Txn) error {
		return txn.Set(entry.Key, entry.Value)
	})

	return err
}

func (b *BadgerDB) UpdateKey(ctx context.Context, oldKey, newKey []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	err := b.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(oldKey)
		if err != nil {
			return err
		}
		itemCopy, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		return txn.Set(newKey, itemCopy)
	})

	return err
}

func (b *BadgerDB) Get(ctx context.Context, key []byte) (*Entry, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	entry := new(Entry)
	err := b.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		valCopy, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}

		entry.Key = key
		entry.Value = valCopy

		return nil
	})

	return entry, err
}

func (b *BadgerDB) LookupAndWrite(ctx context.Context, entry *Entry) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	found := false
	err := b.db.Update(func(txn *badger.Txn) error {
		_, err := txn.Get(entry.Key)
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return txn.Set(entry.Key, entry.Value)
			}
			return err
		}

		found = true
		return nil
	})

	if err != nil {
		return false, err
	}

	return !found, nil
}

func (b *BadgerDB) Delete(ctx context.Context, key []byte) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	err := b.db.Update(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		if err != nil {
			return err
		}
		return txn.Delete(key)
	})

	if err != nil {
		if err == badger.ErrKeyNotFound {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func (b *BadgerDB) List(ctx context.Context, prefix []byte) ([][]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	keys := make([][]byte, 0, 0)
	err := b.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			keyCopy := item.KeyCopy(nil)
			keys = append(keys, keyCopy)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(keys) == 0 {
		return nil, ErrNoRecords
	}

	return keys, nil
}

type Transaction struct {
	db  *badger.DB
	txn *badger.Txn
	ctx context.Context
}

func (t *Transaction) List(ctx context.Context, prefix []byte) ([][]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	keys := make([][]byte, 0, 0)
	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = false
	it := t.txn.NewIterator(opts)
	defer it.Close()
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		item := it.Item()
		keyCopy := item.KeyCopy(nil)
		keys = append(keys, keyCopy)
	}

	if len(keys) == 0 {
		return nil, ErrNoRecords
	}

	return keys, nil
}

func (t *Transaction) Delete(key []byte) (bool, error) {
	select {
	case <-t.ctx.Done():
		return false, t.ctx.Err()
	default:
	}

	_, err := t.txn.Get(key)
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return false, nil
		}
		return false, err
	}

	if err := t.txn.Delete(key); err != nil {
		return false, err
	}

	return true, nil
}

func (t *Transaction) Get(key []byte) (*Entry, error) {
	select {
	case <-t.ctx.Done():
		return nil, t.ctx.Err()
	default:
	}

	entry := new(Entry)
	item, err := t.txn.Get(key)
	if err != nil {
		return nil, err
	}
	valCopy, err := item.ValueCopy(nil)
	if err != nil {
		return nil, err
	}

	entry.Key = key
	entry.Value = valCopy

	return entry, nil
}

func (t *Transaction) Put(entry *Entry) error {
	select {
	case <-t.ctx.Done():
		return t.ctx.Err()
	default:
	}

	_, err := t.txn.Get(entry.Key)
	if err != nil && errors.Is(err, badger.ErrKeyNotFound) {
		return t.txn.Set(entry.Key, entry.Value)
	}
	if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
		return err
	}
	return fmt.Errorf("a record exist for %s key: %w", entry.Key, ErrRecordExist)
}

func (t *Transaction) Commit() error {
	return t.txn.Commit()
}

func (t *Transaction) Rollback() error {
	t.txn.Discard()
	return nil
}

func (t *Transaction) TxnLookupAndWrite(entry *Entry) (bool, error) {
	select {
	case <-t.ctx.Done():
		return false, t.ctx.Err()
	default:
	}

	_, err := t.txn.Get(entry.Key)
	if err != nil {
		if err == badger.ErrKeyNotFound {
			if err := t.txn.Set(entry.Key, entry.Value); err != nil {
				return false, fmt.Errorf("unable to write entry value: %w", err)
			}
			return true, nil
		}
		return false, fmt.Errorf("unable to get entry key: %w", err)
	}
	return false, nil
}

func (t *Transaction) Reset(rw bool) {
	t.txn = t.db.NewTransaction(rw)
}

func (b *BadgerDB) NewTransaction(ctx context.Context, rw bool) *Transaction {
	return &Transaction{
		db:  b.db,
		txn: b.db.NewTransaction(rw),
		ctx: ctx,
	}
}

func (b *BadgerDB) Close() error {
	b.gcInterval.Stop()
	if err := b.seq.Release(); err != nil {
		b.logger.Log(hclog.Error, err.Error())
	}
	return b.db.Close()
}

type DbWriter struct {
	batch   *badger.WriteBatch
	version uint64
	seq     int
	ttl     time.Duration
}

// Write on db using a single transaction batch. Write is not
// concurrent safe.
func (w *DbWriter) Write(p []byte) (int, error) {
	key := fmt.Sprintf("%d/%d/chunk", w.version, w.seq)
	fmt.Println(key)
	e := badger.NewEntry([]byte(key), p).WithTTL(w.ttl)
	if err := w.batch.SetEntry(e); err != nil {
		return 0, err
	}
	w.seq++
	return len(p), nil
}

func (w *DbWriter) SetHash(hash []byte) error {
	key := fmt.Sprintf("%d/%d/hash", w.version, w.seq)
	fmt.Println(key)
	e := badger.NewEntry([]byte(key), hash).WithTTL(w.ttl)
	return w.batch.SetEntry(e)
}

func (w *DbWriter) Commit() error {
	return w.batch.Flush()
}

func (b *BadgerDB) NewBatchWriter(ttl time.Duration) (*DbWriter, error) {
	version, err := b.seq.Next()
	if err != nil {
		return nil, err
	}

	batch := b.db.NewWriteBatch()

	return &DbWriter{
		batch:   batch,
		version: version,
		ttl:     ttl,
	}, nil
}

func NewBadgerDB(config *BadgerConfig, logger hclog.Logger) (*BadgerDB, error) {
	opt := badger.DefaultOptions(config.Path).
		WithInMemory(config.InMemory).
		WithLoggingLevel(badger.ERROR)
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
			err := db.RunValueLogGC(0.5)
			if err == nil {
				logger.Trace("GC run successfully")
				goto again
			}
		}
	}()

	return &BadgerDB{
		db:         db,
		seq:        seq,
		gcInterval: gcInterval,
		logger:     logger,
		config:     config,
	}, nil
}
