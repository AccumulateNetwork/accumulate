// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"slices"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// Find searches for the closest index entry to the target.
func (x *Log[V]) Find(target *record.Key) Query[V] {
	b, err := x.getHead().Get()
	if err != nil {
		return rejectNotFound[V](err)
	}

	flat := b.Level == 0
	for {
		// Find the target entry (binary search)
		i, exact := slices.BinarySearchFunc(b.Entries, target, func(e *Entry[V], t *record.Key) int {
			return e.Key.Compare(t)
		})

		switch {
		case i == 0 && !exact:
			// The target key comes before the first entry
			return &beforeResult[V]{x, target, flat}

		case b.Level == 0 && exact:
			// Found an exact match
			return entry(b.Entries[i])

		case b.Level == 0:
			// The target comes between I-1 and I, or after the end if I ==
			// len(Entries)
			return &closestResult[V]{x, target, b, i - 1}
		}

		// Adjust the index if the match is not exact
		if !exact {
			i--
		}

		// We need to go down a level
		b, err = x.getBlock(b.Level-1, b.Entries[i].Index).Get()
		if err != nil {
			return rejectNotFound[V](err)
		}
	}
}

type beforeResult[V any] struct {
	x      *Log[V]
	target *record.Key
	flat   bool
}

func (r *beforeResult[V]) Before() QueryResult[V] {
	return notFound[V](r.target)
}

func (r *beforeResult[V]) Exact() QueryResult[V] {
	return notFound[V](r.target)
}

func (r *beforeResult[V]) After() QueryResult[V] {
	var b *Block[V]
	var err error
	if r.flat {
		// If the index is flat, get the head
		b, err = r.x.getHead().Get()
	} else {
		// If the index has depth, get the (0, 0) block
		b, err = r.x.getBlock(0, 0).Get()
	}
	if err != nil {
		return rejectNotFound[V](err)
	}

	// If there are no entries, return not found
	if len(b.Entries) == 0 {
		return notFound[V](r.target)
	}

	// Return the first entry
	return entry(b.Entries[0])
}

type closestResult[V any] struct {
	x      *Log[V]
	target *record.Key
	block  *Block[V]
	index  int
}

func (r *closestResult[V]) Before() QueryResult[V] {
	return entry(r.block.Entries[r.index])
}

func (r *closestResult[V]) Exact() QueryResult[V] {
	return notFound[V](r.target)
}

func (r *closestResult[V]) After() QueryResult[V] {
	// If there's a next entry, return it
	if r.index+1 < len(r.block.Entries) {
		return entry(r.block.Entries[r.index+1])
	}

	// If the block is not full it must be the last block, so return not found
	if len(r.block.Entries) < int(r.x.blockSize) {
		return notFound[V](r.target)
	}

	// Look for the next block (at the bottom level)
	next, err := r.x.getBlock(0, r.block.Index+1).Get()
	switch {
	case err == nil:
		// Return its first entry
		return entry(next.Entries[0])

	case errors.Is(err, errors.NotFound):
		// There is no next block, return not found
		return notFound[V](r.target)

	default:
		// Unknown error
		return errResult[V](err)
	}
}

func entry[V any](e *Entry[V]) ValueResult[V] {
	return value(e.Key, e.Value)
}
