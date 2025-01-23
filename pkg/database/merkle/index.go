// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

import (
	"sort"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// NewChainIndex constructs a new ChainIndex data model object. ChainIndex
// supports appending keys in ascending order, e.g. as the chain grows. It does
// not allow adding keys that would sort before or between existing items.
func NewChainIndex(logger log.Logger, store database.Store, key *record.Key) *ChainIndex {
	x := new(ChainIndex)
	x.logger.Set(logger)
	x.store = store
	x.key = key
	x.blockSize = 1 << 12 // 4096
	return x
}

// Last returns the last index entry.
func (x *ChainIndex) Last() (*record.Key, uint64, error) {
	b, err := x.getHead().Get()
	if err != nil {
		return nil, 0, notFoundIsNotOk(err)
	}
	if len(b.Entries) == 0 {
		return nil, 0, errors.NotFound.With("index is empty")
	}
	for b.Level > 0 {
		b, err = x.getBlock(b.Level-1, b.Entries[len(b.Entries)-1].Index).Get()
		if err != nil {
			return nil, 0, notFoundIsNotOk(err)
		}
	}
	e := b.Entries[len(b.Entries)-1]
	return e.Key, e.Index, nil
}

// Append appends a (key, index) entry into the chain index. Append returns an
// error if the key does not come after all other entries.
func (x *ChainIndex) Append(key *record.Key, index uint64) error {
	new, err := x.append1(x.getHead(), &chainIndexEntry{Key: key, Index: index})
	if new == nil || err != nil {
		return err
	}

	// We need a new layer
	old, err := x.getHead().Get()
	if err != nil {
		return notFoundIsNotOk(err)
	}

	// Move the old head
	err = x.getBlock(old.Level, 0).Put(old)
	if err != nil {
		return err
	}

	// Create a new head
	return x.getHead().Put(&chainIndexBlock{
		Level: old.Level + 1,
		Index: 0,
		Entries: []*chainIndexEntry{
			{Index: 0, Key: old.Entries[0].Key},
			{Index: 1, Key: new.Entries[0].Key},
		},
	})
}

func (x *ChainIndex) append1(record values.Value[*chainIndexBlock], e *chainIndexEntry) (*chainIndexBlock, error) {
	// Load the block
	b, err := record.Get()
	if err != nil {
		return nil, notFoundIsNotOk(err)
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
	return x.append2(record, &chainIndexEntry{Index: new.Index, Key: new.Entries[0].Key})
}

func (x *ChainIndex) append2(record values.Value[*chainIndexBlock], e *chainIndexEntry) (*chainIndexBlock, error) {
	b, err := record.Get()
	if err != nil {
		return nil, notFoundIsNotOk(err)
	}

	// Add to this block?
	if len(b.Entries) < int(x.blockSize) {
		b.Entries = append(b.Entries, e)
		return nil, record.Put(b)
	}

	// Create a new block
	new := &chainIndexBlock{
		Level:   b.Level,
		Index:   b.Index + 1,
		Entries: []*chainIndexEntry{e},
	}
	return new, x.getBlock(new.Level, new.Index).Put(new)
}

// Find searches for the closest index entry to the target.
func (x *ChainIndex) Find(target *record.Key) ChainSearchResult {
	b, err := x.getHead().Get()
	if err != nil {
		return errorResult{notFoundIsNotOk(err)}
	}

	flat := b.Level == 0
	for {
		// Find the target entry (binary search)
		i := sort.Search(len(b.Entries), func(i int) bool {
			return target.Compare(b.Entries[i].Key) <= 0
		})

		// Is the match exact?
		exact := i < len(b.Entries) && b.Entries[i].Key.Equal(target)

		switch {
		case i == 0 && !exact:
			// The target key comes before the first entry
			return &beforeResult{x, target, flat}

		case b.Level == 0 && exact:
			// Found an exact match
			return &exactResult{b.Entries[i]}

		case b.Level == 0:
			// The target comes between I-1 and I, or after the end if I ==
			// len(Entries)
			return &closestResult{x, target, b, i - 1}
		}

		// Adjust the index if the match is not exact
		if !exact {
			i--
		}

		// We need to go down a level
		b, err = x.getBlock(b.Level-1, b.Entries[i].Index).Get()
		if err != nil {
			return errorResult{notFoundIsNotOk(err)}
		}
	}
}

func notFoundIsNotOk(err error) error {
	if !errors.Is(err, errors.NotFound) {
		return err
	}

	// Escalate NotFound to InternalError in cases where NotFound indicates a
	// corrupted index. For example, if the head references a block and that
	// block doesn't exist, that's a problem.
	return errors.InternalError.WithFormat("corrupted index: %w", err)
}

type ChainSearchResult interface {
	// Before returns the target entry if the match was exact, the entry before
	// the target if one exists, or an error result.
	Before() ChainSearchResult2

	// Exact returns the target entry if the match was exact, or an error
	// result.
	Exact() ChainSearchResult2

	// After returns the target entry if the match was exact, the entry after
	// the target if one exists, or an error result.
	After() ChainSearchResult2
}

type ChainSearchResult2 interface {
	// Err returns an error if the search failed.
	Err() error

	// Index returns the index of the target entry.
	Index() (uint64, error)

	// TODO Add Next() (ChainSearchResult2, bool) that returns the next entry in
	// the index.
}

type beforeResult struct {
	x      *ChainIndex
	target *record.Key
	flat   bool
}

func (r *beforeResult) Before() ChainSearchResult2 {
	return errorResult{(*database.NotFoundError)(r.target)}
}

func (r *beforeResult) Exact() ChainSearchResult2 {
	return errorResult{(*database.NotFoundError)(r.target)}
}

func (r *beforeResult) After() ChainSearchResult2 {
	var b *chainIndexBlock
	var err error
	if r.flat {
		// If the index is flat, get the head
		b, err = r.x.getHead().Get()
	} else {
		// If the index has depth, get the (0, 0) block
		b, err = r.x.getBlock(0, 0).Get()
	}
	if err != nil {
		return errorResult{notFoundIsNotOk(err)}
	}

	// If there are no entries, return not found
	if len(b.Entries) == 0 {
		return errorResult{(*database.NotFoundError)(r.target)}
	}

	// Return the first entry
	return &exactResult{b.Entries[0]}
}

type exactResult struct {
	entry *chainIndexEntry
}

func (r *exactResult) Before() ChainSearchResult2 { return r }
func (r *exactResult) Exact() ChainSearchResult2  { return r }
func (r *exactResult) After() ChainSearchResult2  { return r }

func (r *exactResult) Err() error             { return nil }
func (r *exactResult) Index() (uint64, error) { return r.entry.Index, nil }

type closestResult struct {
	x      *ChainIndex
	target *record.Key
	block  *chainIndexBlock
	index  int
}

func (r *closestResult) Before() ChainSearchResult2 {
	return &exactResult{r.block.Entries[r.index]}
}

func (r *closestResult) Exact() ChainSearchResult2 {
	return errorResult{(*database.NotFoundError)(r.target)}
}

func (r *closestResult) After() ChainSearchResult2 {
	// If there's a next entry, return it
	if r.index+1 < len(r.block.Entries) {
		return &exactResult{r.block.Entries[r.index+1]}
	}

	// If the block is not full it must be the last block, so return not found
	if len(r.block.Entries) < int(r.x.blockSize) {
		return errorResult{(*database.NotFoundError)(r.target)}
	}

	// Look for the next block (at the bottom level)
	next, err := r.x.getBlock(0, r.block.Index+1).Get()
	switch {
	case err == nil:
		// Return its first entry
		return &exactResult{next.Entries[0]}

	case errors.Is(err, errors.NotFound):
		// There is no next block, return not found
		return errorResult{(*database.NotFoundError)(r.target)}

	default:
		// Unknown error
		return errorResult{err}
	}
}

type errorResult struct {
	err error
}

func (r errorResult) Before() ChainSearchResult2 { return r }
func (r errorResult) Exact() ChainSearchResult2  { return r }
func (r errorResult) After() ChainSearchResult2  { return r }

func (r errorResult) Err() error             { return r.err }
func (r errorResult) Index() (uint64, error) { return 0, r.err }
