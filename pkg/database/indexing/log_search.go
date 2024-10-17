// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"slices"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// Find searches for the closest index entry to the target.
func (x *Log[V]) Find(target *record.Key) Query[V] {
	r, err := x.find(target)
	if err != nil {
		return err
	}

	switch {
	case r.exact:
		// Found an exact match
		return entry(r.block.Entries[r.index])
	case r.index == 0:
		// The target key comes before the first entry
		return &beforeResult[V]{x, target, r.flat}
	default:
		// The target comes between I-1 and I, or after the end if I ==
		// len(Entries)
		return &closestResult[V]{x, target, r.block, r.index - 1}
	}
}

func (x *Log[V]) find(target *record.Key) (findResult[V], queryError[V]) {
	var r findResult[V]
	var first bool
	var err error
	r.record = x.getHead()
	for {
		r.block, err = r.record.Get()
		if err != nil {
			return r, rejectNotFound[V](err)
		}

		// If the highest level is zero, the index is flat
		if first {
			first = false
			r.flat = r.block.Level == 0
		}

		// Find the target entry (binary search)
		r.index, r.exact = slices.BinarySearchFunc(r.block.Entries, target, func(e *Entry[V], t *record.Key) int {
			return e.Key.Compare(t)
		})

		// If we reached the bottom level or the target is before the first
		// entry, return
		if r.block.Level == 0 || r.index == 0 && !r.exact {
			return r, nil
		}

		// Adjust the index if the match is not exact
		if !r.exact {
			r.index--
		}

		// We need to go down a level
		r.record = x.getBlock(r.block.Level-1, r.block.Entries[r.index].Index)
	}
}

type findResult[V any] struct {
	flat   bool
	record values.Value[*Block[V]]
	block  *Block[V]
	index  int
	exact  bool
}

type queryError[V any] interface {
	Query[V]
	error
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
