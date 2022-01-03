package batch

import (
	"sync"

	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
)

const debug = 0 // | debugGet | debugGetValue | debugPut | debugPutValue

const (
	debugGet = 1 << iota
	debugGetValue
	debugPut
	debugPutValue
)

type Batchable interface {
	Get(storage.Key) ([]byte, error)
	EndBatch(map[storage.Key][]byte) error
}

type Batch struct {
	db     Batchable
	mu     *sync.RWMutex
	values map[storage.Key][]byte
	logger storage.Logger
}

var _ storage.KeyValueTxn = (*Batch)(nil)

func New(db Batchable, logger storage.Logger) *Batch {
	return &Batch{
		db:     db,
		mu:     new(sync.RWMutex),
		values: map[storage.Key][]byte{},
		logger: logger,
	}
}

func (b *Batch) logDebug(msg string, keyVals ...interface{}) {
	if b.logger != nil {
		b.logger.Debug(msg, keyVals...)
	}
}

func (b *Batch) Put(key storage.Key, value []byte) {
	if debug&debugPut != 0 {
		b.logDebug("Put", "key", key)
	}
	if debug&debugPutValue != 0 {
		b.logDebug("Put", "value", logging.AsHex(value))
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.values[key] = value
}

func (b *Batch) PutAll(values map[storage.Key][]byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for k, v := range values {
		b.values[k] = v
	}
}

func (b *Batch) Get(key storage.Key) (v []byte, err error) {
	if debug&debugGet != 0 {
		b.logDebug("Get", "key", key)
	}
	if debug&debugGetValue != 0 {
		defer func() {
			if err != nil {
				b.logDebug("Get", "error", err)
			} else {
				b.logDebug("Get", "value", logging.AsHex(v))
			}
		}()
	}

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
