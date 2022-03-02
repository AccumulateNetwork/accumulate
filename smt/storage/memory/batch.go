package memory

import (
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type Batchable interface {
	Get(storage.Key) ([]byte, error)
	EndBatch(map[storage.Key][]byte) error
}

type Batch struct {
	db     Batchable
	mu     *sync.RWMutex
	values map[storage.Key][]byte
}

var _ storage.KeyValueTxn = (*Batch)(nil)

func (b *Batch) Put(key storage.Key, value []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.values[key] = value
	return nil
}

func (b *Batch) PutAll(values map[storage.Key][]byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for k, v := range values {
		b.values[k] = v
	}
	return nil
}

func (b *Batch) Get(key storage.Key) (v []byte, err error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	v, ok := b.values[key]
	if !ok {
		return b.db.Get(key)
	}

	// Return a copy. Otherwise the caller could change it, and that would
	// change what's in the cache.
	u := make([]byte, len(v))
	copy(u, v)
	return u, nil
}

func (b *Batch) Commit() error {
	b.mu.Lock()
	values := b.values
	b.values = nil // Prevent reuse
	b.mu.Unlock()

	return b.db.EndBatch(values)
}

func (b *Batch) Discard() {
	b.mu.Lock()
	b.values = nil // Prevent reuse
	b.mu.Unlock()
}

func (b *Batch) Copy() *Batch {
	c := new(Batch)
	c.db = b.db
	c.mu = new(sync.RWMutex)
	c.values = make(map[storage.Key][]byte, len(b.values))

	for k, v := range b.values {
		c.values[k] = v
	}
	return c
}
