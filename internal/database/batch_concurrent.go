// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"fmt"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// A Batch is not safe for concurrent use: records and values memoize lazily
// in plain maps, so even a READ from a child batch mutates its parent. Batch
// therefore cannot simply be shared across goroutines — but sibling child
// batches CAN run concurrently if every touch of the shared parent is
// serialized. syncStore is that choke point: each child memoizes records and
// values in its own maps, so after the first (locked) pull-through of a
// record, a child's reads and writes are lock-free and private (#4145).
type syncStore struct {
	mu    *sync.Mutex
	inner database.Store
}

func (s syncStore) GetValue(key *record.Key, value database.Value) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.GetValue(key, value)
}

func (s syncStore) PutValue(key *record.Key, value database.Value) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.PutValue(key, value)
}

// Unwrap returns the parent batch's record layer, so the BPT's
// commitUpdatesDirect pushes a concurrent child's pending BPT updates into
// the parent — exactly as a plain child batch does — instead of refusing
// ("cannot determine how the BPT should be committed") and killing the
// block. Only Commit reaches this, and BeginConcurrent's contract serializes
// commits, so no shard goroutine is running when the parent is touched here.
func (s syncStore) Unwrap() database.Record {
	return s.inner.(values.RecordStore).Record
}

// BeginConcurrent starts a child batch whose access to THIS batch is
// serialized by mu. Children created with the same mutex may execute
// concurrently with each other; the parent must not be used directly (by
// anyone) until every concurrent child is committed or discarded, and the
// commits themselves must be serialized by the caller — Commit writes into
// the parent through the same locked store, but the caller almost always
// wants a deterministic commit order anyway.
func (b *Batch) BeginConcurrent(mu *sync.Mutex, writable bool) *Batch {
	if writable && !b.writable {
		b.logger.Info("Attempted to create a writable batch from a read-only batch")
	}

	// Under mu: creating a child MUTATES the parent, and concurrent
	// creation was a real data race (#4149).
	mu.Lock()
	b.nextChildId++
	id := b.nextChildId
	mu.Unlock()

	c := new(Batch)
	c.id = fmt.Sprintf("%s.%d", b.id, id)
	c.observer = b.observer
	c.writable = b.writable && writable
	c.parent = b
	c.logger = b.logger
	c.store = syncStore{mu, values.RecordStore{Record: b}}
	return c
}
