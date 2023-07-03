// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package badger

import (
	"bytes"
	"sync"

	"github.com/dgraph-io/badger"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type Batch struct {
	db       *DB
	txn      *badger.Txn
	writable bool
	prefix   string
	cache    map[string][]byte
}

var _ storage.KeyValueTxn = (*Batch)(nil)

func (db *DB) Begin(writable bool) storage.KeyValueTxn {
	return db.BeginWithPrefix(writable, "")
}

func (db *DB) BeginWithPrefix(writable bool, prefix string) storage.KeyValueTxn {
	b := new(Batch)
	b.db = db
	b.txn = db.badgerDB.NewTransaction(false)
	b.writable = writable
	b.prefix = prefix
	if db.logger == nil {
		return b
	}
	return &storage.DebugBatch{Batch: b, Logger: db.logger, Writable: writable}
}

func (b *Batch) Begin(writable bool) storage.KeyValueTxn {
	return b.BeginWithPrefix(writable, "")
}

func (b *Batch) BeginWithPrefix(writable bool, prefix string) storage.KeyValueTxn {
	if !b.writable || !writable {
		return memory.NewBatch(prefix, b.get, nil)
	}
	return memory.NewBatch(prefix, b.get, b.commit)
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

	if b.cache == nil {
		b.cache = map[string][]byte{}
	}
	k := b.makeKey("", key)
	b.cache[k] = value
	return nil
}

func (b *Batch) PutAll(values map[storage.Key][]byte) error {
	if l, err := b.lock(); err != nil {
		return err
	} else {
		defer l.Unlock()
	}

	if b.cache == nil {
		b.cache = map[string][]byte{}
	}
	for k, v := range values {
		k := b.makeKey("", k)
		b.cache[k] = v
	}

	return nil
}

func (b *Batch) commit(values map[memory.PrefixedKey][]byte) error {
	if l, err := b.lock(); err != nil {
		return err
	} else {
		defer l.Unlock()
	}

	if b.cache == nil {
		b.cache = map[string][]byte{}
	}
	for k, v := range values {
		k := b.makeKey(k.Prefix, k.Key)
		b.cache[k] = v
	}

	return nil
}

func (b *Batch) makeKey(prefix string, key storage.Key) string {
	k := new(bytes.Buffer)
	k.Grow(len(b.prefix) + len(prefix) + 32)
	k.WriteString(b.prefix)
	k.WriteString(prefix)
	k.Write(key[:])
	return k.String()
}

func (b *Batch) Get(key storage.Key) (v []byte, err error) {
	k := b.makeKey("", key)
	if v, ok := b.cache[k]; ok {
		return v, nil
	}
	return b.get("", key)
}

func (b *Batch) get(prefix string, key storage.Key) (v []byte, err error) {
	if l, err := b.db.lock(false); err != nil {
		return nil, err
	} else {
		defer l.Unlock()
	}

	item, err := b.txn.Get([]byte(b.makeKey(prefix, key)))
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, badger.ErrKeyNotFound):
		return nil, errors.NotFound.WithFormat("key %s not found", key)
	default:
		return nil, err
	}

	v, err = item.ValueCopy(nil)
	// If we didn't find the value, return ErrNotFound
	if errors.Is(err, badger.ErrKeyNotFound) {
		return nil, errors.NotFound.WithFormat("key %v not found", key)
	}

	return v, nil
}

func (b *Batch) Commit() error {
	if l, err := b.lock(); err != nil {
		return err
	} else {
		defer l.Unlock()
	}
	b.txn.Discard()

	wb := b.db.badgerDB.NewWriteBatch()
	for k, v := range b.cache {
		err := wb.Set([]byte(k), v)
		if err != nil {
			return err
		}
	}
	return wb.Flush()
}

func (b *Batch) Discard() {
	b.txn.Discard()
}
