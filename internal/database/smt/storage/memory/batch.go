// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package memory

import (
	"encoding/hex"
	"fmt"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type GetFunc func(string, storage.Key) ([]byte, error)
type CommitFunc func(map[PrefixedKey][]byte) error

type PrefixedKey struct {
	Prefix string
	Key    storage.Key
}

type Batch struct {
	getfn         GetFunc
	commitfn      CommitFunc
	mu            *sync.RWMutex
	values        map[PrefixedKey][]byte
	debugWriteLog []writeLogEntry
	prefix        string
}

type writeLogEntry struct {
	key    PrefixedKey
	keyStr string
	value  string
}

var _ storage.KeyValueTxn = (*Batch)(nil)

func NewBatch(prefix string, get GetFunc, commit CommitFunc) storage.KeyValueTxn {
	b := &Batch{
		getfn:    get,
		commitfn: commit,
		mu:       new(sync.RWMutex),
		values:   map[PrefixedKey][]byte{},
		prefix:   prefix,
	}
	return b
}

func (k PrefixedKey) String() string { return k.Prefix + k.Key.String() }

func (b *Batch) write(k PrefixedKey, v []byte) {
	b.values[k] = v
	if debugLogWrites {
		b.debugWriteLog = append(b.debugWriteLog, writeLogEntry{
			key:    k,
			keyStr: k.String(),
			value:  hex.EncodeToString(v),
		})
	}
}

func (db *DB) Begin(writable bool) storage.KeyValueTxn {
	return db.BeginWithPrefix(writable, "")
}

func (db *DB) BeginWithPrefix(writable bool, prefix string) storage.KeyValueTxn {
	var b storage.KeyValueTxn
	if writable {
		b = NewBatch(prefix, db.get, db.commit)
	} else {
		b = NewBatch(prefix, db.get, nil)
	}
	if db.logger == nil {
		return b
	}
	return &storage.DebugBatch{Batch: b, Logger: db.logger, Writable: writable}
}

func (b *Batch) Begin(writable bool) storage.KeyValueTxn {
	return b.BeginWithPrefix(writable, "")
}

func (b *Batch) BeginWithPrefix(writable bool, prefix string) storage.KeyValueTxn {
	if b.commitfn == nil || !writable {
		return NewBatch(prefix, b.get, nil)
	}
	return NewBatch(prefix, b.get, b.commit)
}

func (b *Batch) Put(key storage.Key, value []byte) error {
	return b.put("", key, value)
}

func (b *Batch) put(prefix string, key storage.Key, value []byte) error {
	if b.commitfn == nil {
		return fmt.Errorf("transaction is not writable")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.write(b.makeKey(prefix, key), value)
	return nil
}

func (b *Batch) PutAll(values map[storage.Key][]byte) error {
	if b.commitfn == nil {
		return fmt.Errorf("transaction is not writable")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for k, v := range values {
		b.write(b.makeKey("", k), v)
	}
	return nil
}

func (b *Batch) commit(values map[PrefixedKey][]byte) error {
	if b.commitfn == nil {
		return fmt.Errorf("transaction is not writable")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for k, v := range values {
		b.write(k, v)
	}
	return nil
}

func (b *Batch) makeKey(prefix string, key storage.Key) PrefixedKey {
	return PrefixedKey{b.prefix + prefix, key}
}

func (b *Batch) Get(key storage.Key) (v []byte, err error) {
	return b.get("", key)
}

func (b *Batch) get(prefix string, key storage.Key) (v []byte, err error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	k := b.makeKey(prefix, key)
	v, ok := b.values[k]
	if ok {
		// Return a copy. Otherwise the caller could change it, and that would
		// change what's in the cache.
		u := make([]byte, len(v))
		copy(u, v)
		return u, nil
	}

	v, err = b.getfn(k.Prefix, k.Key)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return v, nil
}

func (b *Batch) Commit() error {
	b.mu.Lock()
	values := b.values
	b.values = nil // Prevent reuse
	b.mu.Unlock()

	if b.commitfn == nil {
		return nil
	}

	return b.commitfn(values)
}

func (b *Batch) Discard() {
	b.mu.Lock()
	b.values = nil // Prevent reuse
	b.mu.Unlock()
}

func (b *Batch) Copy() *Batch {
	c := new(Batch)
	c.getfn = b.getfn
	c.commitfn = b.commitfn
	c.mu = new(sync.RWMutex)
	c.values = make(map[PrefixedKey][]byte, len(b.values))

	for k, v := range b.values {
		c.values[k] = v
	}
	return c
}
