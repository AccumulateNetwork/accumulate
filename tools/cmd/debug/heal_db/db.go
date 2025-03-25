// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package healdb provides an in-memory database implementation for the heal_synth tool.
package healdb

import (
	"fmt"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// KeySize is the size of keys in the database (32 bytes)
const KeySize = 32

// Key is a 32-byte key used in the database
type Key [KeySize]byte

// Database is the interface for basic key-value operations
type Database interface {
	// Get retrieves a value by its key
	Get(key Key) (interface{}, error)
	
	// Put stores a value with a key
	Put(key Key, value interface{}) error
	
	// Delete removes a key-value pair
	Delete(key Key) error
	
	// Begin starts a new batch/transaction
	Begin(writable bool) Batch
}

// Batch is the interface for batch operations
type Batch interface {
	// Get retrieves a value by its key within the batch
	Get(key Key) (interface{}, error)
	
	// Put stages a value to be written
	Put(key Key, value interface{}) error
	
	// Delete stages a key for deletion
	Delete(key Key) error
	
	// Commit applies all changes to the database
	Commit() error
	
	// Discard discards all pending changes
	Discard()
}

// MemoryDB implements the Database interface with an in-memory store
type MemoryDB struct {
	data   map[Key]interface{}
	mutex  sync.RWMutex
	logger Logger
}

// MemoryBatch implements the Batch interface for MemoryDB
type MemoryBatch struct {
	db        *MemoryDB
	writable  bool
	changes   map[Key]batchEntry
	committed bool
	mutex     sync.RWMutex
}

// batchEntry represents a single change in a batch
type batchEntry struct {
	value   interface{}
	deleted bool
}

// Logger interface for database logging
type Logger interface {
	Debug(format string, args ...interface{})
	Info(format string, args ...interface{})
	Error(format string, args ...interface{})
}

// NotFoundError is returned when a key is not found in the database
type NotFoundError struct {
	Key Key
}

func (e NotFoundError) Error() string {
	return fmt.Sprintf("key not found: %X", e.Key)
}

// Pre-defined errors
var (
	ErrBatchClosed   = fmt.Errorf("batch is closed")
	ErrReadOnlyBatch = fmt.Errorf("batch is read-only")
	ErrInvalidKeyType = fmt.Errorf("invalid key type")
	ErrNilValue      = fmt.Errorf("nil value not allowed")
)

// NewDatabase creates a new empty in-memory database
func NewDatabase() *MemoryDB {
	return &MemoryDB{
		data: make(map[Key]interface{}),
	}
}

// Get retrieves a value by its key
func (db *MemoryDB) Get(key Key) (interface{}, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()
	
	value, ok := db.data[key]
	if !ok {
		return nil, NotFoundError{Key: key}
	}
	return value, nil
}

// Put stores a value with a key
func (db *MemoryDB) Put(key Key, value interface{}) error {
	if value == nil {
		return ErrNilValue
	}
	
	db.mutex.Lock()
	defer db.mutex.Unlock()
	
	db.data[key] = value
	return nil
}

// Delete removes a key-value pair
func (db *MemoryDB) Delete(key Key) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	
	delete(db.data, key)
	return nil
}

// Begin starts a new batch/transaction
func (db *MemoryDB) Begin(writable bool) Batch {
	return &MemoryBatch{
		db:       db,
		writable: writable,
		changes:  make(map[Key]batchEntry),
	}
}

// WithLogger sets a logger for database operations
func (db *MemoryDB) WithLogger(logger Logger) *MemoryDB {
	db.logger = logger
	return db
}

// Get retrieves a value by its key within the batch
func (b *MemoryBatch) Get(key Key) (interface{}, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	
	if b.committed {
		return nil, ErrBatchClosed
	}
	
	// Check if the key exists in the batch changes
	if entry, ok := b.changes[key]; ok {
		if entry.deleted {
			return nil, NotFoundError{Key: key}
		}
		return entry.value, nil
	}
	
	// Fall back to the database
	return b.db.Get(key)
}

// Put stages a value to be written
func (b *MemoryBatch) Put(key Key, value interface{}) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	
	if b.committed {
		return ErrBatchClosed
	}
	
	if !b.writable {
		return ErrReadOnlyBatch
	}
	
	if value == nil {
		return ErrNilValue
	}
	
	b.changes[key] = batchEntry{
		value:   value,
		deleted: false,
	}
	
	return nil
}

// Delete stages a key for deletion
func (b *MemoryBatch) Delete(key Key) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	
	if b.committed {
		return ErrBatchClosed
	}
	
	if !b.writable {
		return ErrReadOnlyBatch
	}
	
	b.changes[key] = batchEntry{
		value:   nil,
		deleted: true,
	}
	
	return nil
}

// Commit applies all changes to the database
func (b *MemoryBatch) Commit() error {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	
	if b.committed {
		return ErrBatchClosed
	}
	
	// Mark as committed immediately to prevent further operations
	b.committed = true
	
	// For read-only batches, just mark as committed and return
	if !b.writable {
		return nil
	}
	
	// Apply changes to the database
	b.db.mutex.Lock()
	defer b.db.mutex.Unlock()
	
	for key, entry := range b.changes {
		if entry.deleted {
			delete(b.db.data, key)
		} else {
			b.db.data[key] = entry.value
		}
	}
	
	return nil
}

// Discard discards all pending changes
func (b *MemoryBatch) Discard() {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	
	b.committed = true
	b.changes = nil
}

// KeyFromURL converts a URL to a 32-byte key hash
func KeyFromURL(u *url.URL) Key {
	hash := u.Hash32()
	return hash
}

// KeyFromString converts a string to a 32-byte key hash
func KeyFromString(s string) Key {
	key := record.NewKey(s)
	hash := key.Hash()
	var result Key
	copy(result[:], hash[:])
	return result
}

// KeyFromValues creates a composite key from multiple values
func KeyFromValues(values ...interface{}) Key {
	key := record.NewKey(values...)
	hash := key.Hash()
	var result Key
	copy(result[:], hash[:])
	return result
}

// KeyFromBytes creates a key directly from a byte slice
func KeyFromBytes(b []byte) Key {
	var key Key
	if len(b) == KeySize {
		copy(key[:], b)
	} else {
		// Hash the bytes if they're not exactly 32 bytes
		hashedKey := record.NewKey(b).Hash()
		copy(key[:], hashedKey[:])
	}
	return key
}
