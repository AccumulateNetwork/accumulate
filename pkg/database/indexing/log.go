// Copyright 2025 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// NewLog constructs a new Log data model object. Log supports appending keys in
// ascending order, e.g. as the chain grows. It does not allow adding keys that
// would sort before or between existing items.
func NewLog[V any](logger log.Logger, store database.Store, key *record.Key, blockSize uint64) *Log[V] {
	if blockSize == 0 {
		panic("block size must be specified")
	}

	x := new(Log[V])
	x.logger.Set(logger)
	x.store = store
	x.key = key
	x.blockSize = blockSize
	return x
}

// Last returns the last entry.
func (x *Log[V]) Last() (*record.Key, V, error) {
	var z V
	b, err := x.getHead().Get()
	if err != nil {
		return nil, z, rejectNotFound[V](err)
	}
	if len(b.Entries) == 0 {
		return nil, z, errors.NotFound.With("log is empty")
	}
	for b.Level > 0 {
		b, err = x.getBlock(b.Level-1, b.Entries[len(b.Entries)-1].Index).Get()
		if err != nil {
			return nil, z, rejectNotFound[V](err)
		}
	}
	e := b.Entries[len(b.Entries)-1]
	v, err := e.Value.Get()
	return e.Key, v, err
}

// Append appends a (key, value) entry into the chain. Append returns an
// error if the key does not come after all other entries.
func (x *Log[V]) Append(key *record.Key, value V) error {
	new, err := x.append1(x.getHead(), &Entry[V]{Key: key, Value: &Value[V]{value: value, valueOk: true}})
	if new == nil || err != nil {
		return err
	}

	// We need a new layer
	old, err := x.getHead().Get()
	if err != nil {
		return rejectNotFound[V](err)
	}

	// Move the old head
	err = x.getBlock(old.Level, 0).Put(old)
	if err != nil {
		return err
	}

	// Create a new head
	return x.getHead().Put(&Block[V]{
		Level: old.Level + 1,
		Index: 0,
		Entries: []*Entry[V]{
			{Index: 0, Key: old.Entries[0].Key},
			{Index: 1, Key: new.Entries[0].Key},
		},
	})
}

func (x *Log[V]) append1(record values.Value[*Block[V]], e *Entry[V]) (*Block[V], error) {
	// Load the block
	b, err := record.Get()
	if err != nil {
		return nil, rejectNotFound[V](err)
	}

	// Can't go backwards
	if len(b.Entries) > 0 && e.Key.Compare(b.Entries[len(b.Entries)-1].Key) < 0 {
		return nil, errors.NotAllowed.With("cannot index past entries")
	}

	// Are we at the bottom layer?
	if b.Level == 0 {
		return x.append2(record, e)
	}

	// Add to the last child
	last := b.Entries[len(b.Entries)-1]
	new, err := x.append1(x.getBlock(b.Level-1, last.Index), e)
	if new == nil || err != nil {
		return nil, err
	}

	// Deal with the new child
	return x.append2(record, &Entry[V]{Index: new.Index, Key: new.Entries[0].Key})
}

func (x *Log[V]) append2(record values.Value[*Block[V]], e *Entry[V]) (*Block[V], error) {
	b, err := record.Get()
	if err != nil {
		return nil, rejectNotFound[V](err)
	}

	// Add to this block?
	if len(b.Entries) < int(x.blockSize) {
		b.Entries = append(b.Entries, e)
		return nil, record.Put(b)
	}

	// Create a new block
	new := &Block[V]{
		Level:   b.Level,
		Index:   b.Index + 1,
		Entries: []*Entry[V]{e},
	}
	return new, x.getBlock(new.Level, new.Index).Put(new)
}

func (x *Log[V]) All(yield func(QueryResult[V]) bool) {
	x.all(x.getHead(), yield)
}

func (x *Log[V]) all(block values.Value[*Block[V]], yield func(QueryResult[V]) bool) bool {
	b, err := block.Get()
	if err != nil {
		yield(errResult[V](err))
		return false
	}

	if b.Level == 0 {
		for _, e := range b.Entries {
			if !yield(entry(e)) {
				return false
			}
		}
		return true
	}

	for _, e := range b.Entries {
		if !x.all(x.getBlock(b.Level-1, e.Index), yield) {
			return false
		}
	}
	return true
}
