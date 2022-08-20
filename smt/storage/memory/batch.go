package memory

import (
	"encoding/hex"
	"fmt"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type GetFunc func(storage.Key) ([]byte, error)
type CommitFunc func(map[storage.Key][]byte) error

type Batch struct {
	get           GetFunc
	commit        CommitFunc
	mu            *sync.RWMutex
	values        map[storage.Key][]byte
	debugWriteLog []writeLogEntry
}

type writeLogEntry struct {
	key    storage.Key
	keyStr string
	value  string
}

var _ storage.KeyValueTxn = (*Batch)(nil)

func NewBatch(get GetFunc, commit CommitFunc) storage.KeyValueTxn {
	b := &Batch{
		get:    get,
		commit: commit,
		mu:     new(sync.RWMutex),
		values: map[storage.Key][]byte{},
	}
	return b
}

func (db *DB) Begin(writable bool) storage.KeyValueTxn {
	var b storage.KeyValueTxn
	if writable {
		b = NewBatch(db.get, db.commit)
	} else {
		b = NewBatch(db.get, nil)
	}
	if db.logger == nil {
		return b
	}
	return &storage.DebugBatch{Batch: b, Logger: db.logger, Writable: writable}
}

func (b *Batch) Begin(writable bool) storage.KeyValueTxn {
	if b.commit == nil || !writable {
		return NewBatch(b.Get, nil)
	}
	return NewBatch(b.Get, b.PutAll)
}

func (b *Batch) Put(key storage.Key, value []byte) error {
	if b.commit == nil {
		return fmt.Errorf("transaction is not writable")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.values[key] = value
	if debugLogWrites {
		b.debugWriteLog = append(b.debugWriteLog, writeLogEntry{
			key:    key,
			keyStr: key.String(),
			value:  hex.EncodeToString(value),
		})
	}
	return nil
}

func (b *Batch) PutAll(values map[storage.Key][]byte) error {
	if b.commit == nil {
		return fmt.Errorf("transaction is not writable")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for k, v := range values {
		b.values[k] = v
		if debugLogWrites {
			b.debugWriteLog = append(b.debugWriteLog, writeLogEntry{
				key:    k,
				keyStr: k.String(),
				value:  hex.EncodeToString(v),
			})
		}
	}
	return nil
}

func (b *Batch) Get(key storage.Key) (v []byte, err error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	v, ok := b.values[key]
	if ok {
		// Return a copy. Otherwise the caller could change it, and that would
		// change what's in the cache.
		u := make([]byte, len(v))
		copy(u, v)
		return u, nil
	}

	v, err = b.get(key)
	if err != nil {
		return nil, errors.Unknown.Wrap(err)
	}
	return v, nil
}

func (b *Batch) Commit() error {
	b.mu.Lock()
	values := b.values
	b.values = nil // Prevent reuse
	b.mu.Unlock()

	if b.commit == nil {
		return nil
	}

	return b.commit(values)
}

func (b *Batch) Discard() {
	b.mu.Lock()
	b.values = nil // Prevent reuse
	b.mu.Unlock()
}

func (b *Batch) Copy() *Batch {
	c := new(Batch)
	c.get = b.get
	c.commit = b.commit
	c.mu = new(sync.RWMutex)
	c.values = make(map[storage.Key][]byte, len(b.values))

	for k, v := range b.values {
		c.values[k] = v
	}
	return c
}
