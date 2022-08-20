package badger

import (
	"sync"

	"github.com/dgraph-io/badger"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
)

type Batch struct {
	db       *DB
	txn      *badger.Txn
	writable bool
}

var _ storage.KeyValueTxn = (*Batch)(nil)

func (db *DB) Begin(writable bool) storage.KeyValueTxn {
	b := new(Batch)
	b.db = db
	b.txn = db.badgerDB.NewTransaction(writable)
	b.writable = writable
	if db.logger == nil {
		return b
	}
	return &storage.DebugBatch{Batch: b, Logger: db.logger, Writable: writable}
}

func (b *Batch) Begin(writable bool) storage.KeyValueTxn {
	if !b.writable || !writable {
		return memory.NewBatch(b.Get, nil)
	}
	return memory.NewBatch(b.Get, b.PutAll)
}

func (b *Batch) lock() (sync.Locker, error) {
	l, err := b.db.lock(false)
	if err == nil {
		return l, nil
	}

	b.Discard() // Is this a good idea?
	return nil, err
}

func (b *Batch) Put(key storage.Key, value []byte) error {
	if l, err := b.lock(); err != nil {
		return err
	} else {
		defer l.Unlock()
	}

	return b.txn.Set(key[:], value)
}

func (b *Batch) PutAll(values map[storage.Key][]byte) error {
	if l, err := b.lock(); err != nil {
		return err
	} else {
		defer l.Unlock()
	}

	for k, v := range values {
		k := k // See docs/developer/rangevarref.md
		err := b.txn.Set(k[:], v)
		if err != nil {
			return err
		}
	}

	return nil
}

func (b *Batch) Get(key storage.Key) (v []byte, err error) {
	if l, err := b.db.lock(false); err != nil {
		return nil, err
	} else {
		defer l.Unlock()
	}

	item, err := b.txn.Get(key[:])
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, badger.ErrKeyNotFound):
		return nil, errors.NotFound.Format("key %s not found", key)
	default:
		return nil, err
	}

	v, err = item.ValueCopy(nil)
	// If we didn't find the value, return ErrNotFound
	if errors.Is(err, badger.ErrKeyNotFound) {
		return nil, errors.NotFound.Format("key %v not found", key)
	}

	return v, nil
}

func (b *Batch) Commit() error {
	if l, err := b.lock(); err != nil {
		return err
	} else {
		defer l.Unlock()
	}

	return b.txn.Commit()
}

func (b *Batch) Discard() {
	b.txn.Discard()
}
