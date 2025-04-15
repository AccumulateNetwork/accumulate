// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package memdb

import (
	"io"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// Common errors for the MemDB implementation
var (
	ErrDatabaseClosed = errors.BadRequest.With("database is closed")
	ErrBatchClosed    = errors.BadRequest.With("batch is closed")
	ErrReadOnlyBatch  = errors.BadRequest.With("batch is read-only")
)

// MemDB implements an in-memory key-value store for the healing process.
type MemDB struct {
	data     map[[32]byte]valueEntry // Main data store using key hash for lookups
	rwMutex  sync.RWMutex            // Read-write mutex for thread safety
	txnMutex sync.Mutex              // Transaction mutex for serializing write transactions
	closed   bool                    // Flag indicating if the database is closed
}

// valueEntry stores both the value and the original key
type valueEntry struct {
	key   *record.Key
	value []byte
}

// Ensure MemDB implements the required interfaces
var (
	_ keyvalue.Beginner = (*MemDB)(nil)
	_ io.Closer         = (*MemDB)(nil)
)

// New creates a new MemDB instance.
func New() *MemDB {
	return &MemDB{
		data: make(map[[32]byte]valueEntry),
	}
}

// Begin begins a transaction with the specified prefix and write mode.
func (db *MemDB) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	db.rwMutex.RLock()
	defer db.rwMutex.RUnlock()

	if db.closed {
		return &MemDBBatch{
			db:      db,
			closed:  true,
			changes: make(map[[32]byte]entry),
		}
	}

	batch := &MemDBBatch{
		db:       db,
		writable: writable,
		prefix:   prefix,
		changes:  make(map[[32]byte]entry),
	}

	// For read-only transactions, create a snapshot of the current data
	if !writable {
		batch.snapshot = make(map[[32]byte]valueEntry, len(db.data))
		for k, v := range db.data {
			// Make a copy of the value to ensure isolation
			value := make([]byte, len(v.value))
			copy(value, v.value)
			batch.snapshot[k] = valueEntry{
				key:   v.key,
				value: value,
			}
		}
	}

	return batch
}

// Close closes the database and releases resources.
func (db *MemDB) Close() error {
	db.rwMutex.Lock()
	defer db.rwMutex.Unlock()

	if db.closed {
		return nil
	}

	db.closed = true
	db.data = nil
	return nil
}

// entry represents a pending change in a transaction.
type entry struct {
	key    *record.Key // The original key
	value  []byte      // The value (nil if this is a delete operation)
	delete bool        // True if this is a delete operation
}

// MemDBBatch implements a transaction for the MemDB.
type MemDBBatch struct {
	db       *MemDB                // Reference to the database
	writable bool                  // Flag indicating if the transaction is writable
	prefix   *record.Key           // Key prefix for this batch
	snapshot map[[32]byte]valueEntry // Snapshot of data at transaction start (for read-only)
	changes  map[[32]byte]entry    // Pending changes
	closed   bool                  // Flag indicating if the transaction is closed
	parent   *MemDBBatch           // Parent batch for nested transactions
	mu       sync.RWMutex          // Mutex for thread safety within the batch
}

// Ensure MemDBBatch implements the required interfaces
var _ keyvalue.ChangeSet = (*MemDBBatch)(nil)

// Begin begins a nested transaction with the specified prefix and write mode.
func (b *MemDBBatch) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return &MemDBBatch{
			db:      b.db,
			closed:  true,
			changes: make(map[[32]byte]entry),
		}
	}

	// Cannot create a writable batch from a read-only batch
	if writable && !b.writable {
		panic("cannot create a writable batch from a read-only batch")
	}

	// Create a new prefix by combining the current prefix with the new one
	var newPrefix *record.Key
	if b.prefix != nil {
		newPrefix = b.prefix.AppendKey(prefix)
	} else {
		newPrefix = prefix
	}

	// Create the nested batch
	nested := &MemDBBatch{
		db:       b.db,
		writable: writable,
		prefix:   newPrefix,
		changes:  make(map[[32]byte]entry),
		parent:   b, // Set the parent reference
	}

	// Create a snapshot that includes the parent's changes
	nested.snapshot = make(map[[32]byte]valueEntry)

	// First, copy the parent's snapshot (if it exists)
	if b.snapshot != nil {
		for k, v := range b.snapshot {
			value := make([]byte, len(v.value))
			copy(value, v.value)
			nested.snapshot[k] = valueEntry{
				key:   v.key,
				value: value,
			}
		}
	} else {
		// If the parent doesn't have a snapshot, create one from the database
		b.db.rwMutex.RLock()
		for k, v := range b.db.data {
			value := make([]byte, len(v.value))
			copy(value, v.value)
			nested.snapshot[k] = valueEntry{
				key:   v.key,
				value: value,
			}
		}
		b.db.rwMutex.RUnlock()
	}

	// Apply the parent's changes to the snapshot
	for k, e := range b.changes {
		if e.delete {
			delete(nested.snapshot, k)
		} else {
			value := make([]byte, len(e.value))
			copy(value, e.value)
			nested.snapshot[k] = valueEntry{
				key:   e.key,
				value: value,
			}
		}
	}

	return nested
}

// Get retrieves a value from the transaction.
func (b *MemDBBatch) Get(key *record.Key) ([]byte, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return nil, ErrBatchClosed
	}

	// Check if the database is closed
	if b.db.closed {
		return nil, ErrDatabaseClosed
	}

	// Apply the prefix to the key
	if b.prefix != nil && key != nil {
		key = b.prefix.AppendKey(key)
	}

	// Calculate the key hash
	keyHash := key.Hash()

	// Check if the key exists in the pending changes
	if e, ok := b.changes[keyHash]; ok {
		if e.delete {
			return nil, (*database.NotFoundError)(key)
		}
		// Return a copy of the value to ensure isolation
		value := make([]byte, len(e.value))
		copy(value, e.value)
		return value, nil
	}

	// If this is a read-only transaction with a snapshot, check the snapshot
	if b.snapshot != nil {
		if entry, ok := b.snapshot[keyHash]; ok {
			// Return a copy of the value to ensure isolation
			result := make([]byte, len(entry.value))
			copy(result, entry.value)
			return result, nil
		}
		return nil, (*database.NotFoundError)(key)
	}

	// Otherwise, check the database directly
	b.db.rwMutex.RLock()
	defer b.db.rwMutex.RUnlock()

	if b.db.closed {
		return nil, ErrDatabaseClosed
	}

	if entry, ok := b.db.data[keyHash]; ok {
		// Return a copy of the value to ensure isolation
		result := make([]byte, len(entry.value))
		copy(result, entry.value)
		return result, nil
	}

	return nil, (*database.NotFoundError)(key)
}

// Put stores a value in the transaction.
func (b *MemDBBatch) Put(key *record.Key, value []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return ErrBatchClosed
	}

	// Check if the database is closed
	if b.db.closed {
		return ErrDatabaseClosed
	}

	if !b.writable {
		return ErrReadOnlyBatch
	}

	// Apply the prefix to the key
	if b.prefix != nil && key != nil {
		key = b.prefix.AppendKey(key)
	}

	// Make a copy of the value to ensure isolation
	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)

	// Add the change to the pending changes
	b.changes[key.Hash()] = entry{
		key:    key,
		value:  valueCopy,
		delete: false,
	}

	return nil
}

// Delete removes a key-value pair from the transaction.
func (b *MemDBBatch) Delete(key *record.Key) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return ErrBatchClosed
	}

	// Check if the database is closed
	if b.db.closed {
		return ErrDatabaseClosed
	}

	if !b.writable {
		return ErrReadOnlyBatch
	}

	// Apply the prefix to the key
	if b.prefix != nil && key != nil {
		key = b.prefix.AppendKey(key)
	}

	// Add the delete operation to the pending changes
	b.changes[key.Hash()] = entry{
		key:    key,
		value:  nil,
		delete: true,
	}

	return nil
}

// Commit applies the pending changes to the database.
func (b *MemDBBatch) Commit() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return ErrBatchClosed
	}

	if !b.writable {
		b.closed = true
		return nil
	}

	// If this is a nested batch, apply changes to the parent
	if b.parent != nil {
		b.parent.mu.Lock()
		defer b.parent.mu.Unlock()

		// Apply changes to the parent batch
		for keyHash, e := range b.changes {
			b.parent.changes[keyHash] = e
		}

		b.closed = true
		return nil
	}

	// This is a root batch, apply changes to the database
	b.db.txnMutex.Lock()
	defer b.db.txnMutex.Unlock()

	b.db.rwMutex.Lock()
	defer b.db.rwMutex.Unlock()

	if b.db.closed {
		b.closed = true
		return ErrDatabaseClosed
	}

	// Apply the changes to the database
	for _, e := range b.changes {
		keyHash := e.key.Hash()
		if e.delete {
			delete(b.db.data, keyHash)
		} else {
			b.db.data[keyHash] = valueEntry{
				key:   e.key,
				value: e.value,
			}
		}
	}

	b.closed = true
	return nil
}

// Discard abandons the pending changes.
func (b *MemDBBatch) Discard() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closed = true
	b.changes = nil
	b.snapshot = nil
}

// ForEach iterates over each key-value pair in the transaction.
func (b *MemDBBatch) ForEach(fn func(*record.Key, []byte) error) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return ErrBatchClosed
	}

	// Create a map to track which keys we've already processed
	processed := make(map[[32]byte]bool)

	// First, process the pending changes
	for keyHash, e := range b.changes {
		if e.delete {
			// Skip deleted entries
			processed[keyHash] = true
			continue
		}

		// Call the function with the key and value
		err := fn(e.key, e.value)
		if err != nil {
			return err
		}

		// Mark this key as processed
		processed[keyHash] = true
	}

	// If this is a read-only transaction with a snapshot, process the snapshot
	if b.snapshot != nil {
		// Iterate over the snapshot
		for keyHash, entry := range b.snapshot {
			// Skip keys that have already been processed
			if processed[keyHash] {
				continue
			}

			// Call the function with the key and value
			err := fn(entry.key, entry.value)
			if err != nil {
				return err
			}

			// Mark this key as processed
			processed[keyHash] = true
		}
	} else {
		// Otherwise, process the database directly
		b.db.rwMutex.RLock()
		defer b.db.rwMutex.RUnlock()

		if b.db.closed {
			return ErrDatabaseClosed
		}

		// Iterate over the database
		for keyHash, entry := range b.db.data {
			// Skip keys that have already been processed
			if processed[keyHash] {
				continue
			}

			// Call the function with the key and value
			err := fn(entry.key, entry.value)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
