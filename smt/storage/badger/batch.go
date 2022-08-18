package badger

import (
	"bytes"
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
	cache    *map[[32]byte][]byte
}

var _ storage.KeyValueTxn = (*Batch)(nil)

func (db *DB) Begin(writable bool) storage.KeyValueTxn {
	b := new(Batch)
	b.db = db
	b.txn = db.badgerDB.NewTransaction(writable)
	b.writable = writable
	if db.cache == nil {
		db.cache = make(map[[32]byte][]byte)
	}
	b.cache = &db.cache
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

	if bytes.Equal((*b.cache)[key],value){ // If we already have that value, ignore
		return nil
	}

	err := b.txn.Set(key[:], value)
	if err != nil {
		(*b.cache)[key] = value
	}

	return err
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
		(*b.cache)[k] = v
	}

	return nil
}

func (b *Batch) Get(key storage.Key) (v []byte, err error) {
	if l, err := b.db.lock(false); err != nil {
		return nil, err
	} else {
		defer l.Unlock()
	}

	if v, ok := (*b.cache)[key]; ok {
		return v, nil
	}

	item, err := b.txn.Get(key[:])
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, badger.ErrKeyNotFound):
		return nil, errors.NotFound("key %s not found", key)
	default:
		return nil, err
	}

	v, err = item.ValueCopy(nil)
	// If we didn't find the value, return ErrNotFound
	if errors.Is(err, badger.ErrKeyNotFound) {
		return nil, errors.NotFound("key %v not found", key)
	}

	b.cache[key] = v

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
