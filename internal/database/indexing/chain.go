// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var ErrReachedChainEnd = errors.NotFound.With("reached the end of the chain")
var ErrReachedChainStart = errors.NotFound.With("reached the start of the chain")
var ErrTargetDoesNotExist = errors.NotFound.With("target does not exist")

// SearchDirection represents a direction to search along a linear index.
type SearchDirection int

const (
	// SearchComplete is returned when the search is complete.
	SearchComplete SearchDirection = iota

	// SearchForward is returned when the search should proceed forwards along
	// the index.
	SearchForward

	// SearchBackward is returned when the search should proceed backwards along
	// the index.
	SearchBackward
)

// MatchMode determines how results are returned from a search.
type MatchMode int

const (
	// MatchExect returns only an exact match.
	MatchExact MatchMode = iota

	// MatchBefore returns the element before the target if the target cannot be
	// found.
	MatchBefore

	// MatchAfter returns the element after the target if the target cannot be
	// found.
	MatchAfter
)

// IndexChainSearchFunction determines the direction an index chain search should proceed.
type IndexChainSearchFunction func(*protocol.IndexEntry) SearchDirection

// SearchIndexChain2 is a wrapper for [SearchIndexChain] that accepts
// [database.Chain2].
func SearchIndexChain2(chain *database.Chain2, index uint64, mode MatchMode, find IndexChainSearchFunction) (uint64, *protocol.IndexEntry, error) {
	c, err := chain.Get()
	if err != nil {
		return 0, nil, err
	}
	return SearchIndexChain(c, index, mode, find)
}

// SearchIndexChain searches an index chain using the given search function.
// Index chains are ordered, and every search function is monotone over that
// order, so the search runs in O(log n) reads via binary search. The starting
// index only affects which entry of a run of equally-matching entries is
// returned (for range search functions) and the boundary-condition errors,
// preserving the semantics of the original linear walk.
func SearchIndexChain(chain *database.Chain, start uint64, mode MatchMode, find IndexChainSearchFunction) (uint64, *protocol.IndexEntry, error) {
	height := uint64(chain.Height())
	read := func(i uint64) (*protocol.IndexEntry, error) {
		entry := new(protocol.IndexEntry)
		err := chain.EntryAs(int64(i), entry)
		if err != nil {
			return nil, fmt.Errorf("entry %d %w", i, err)
		}
		return entry, nil
	}

	// Read the starting entry first to preserve the original behavior: a match
	// at the start returns immediately, and the boundary special cases depend
	// on the direction the walk would have taken from the start.
	startEntry, err := read(start)
	if err != nil {
		return 0, nil, err
	}
	switch find(startEntry) {
	case SearchComplete:
		return start, startEntry, nil
	case SearchBackward:
		if start == 0 && mode == MatchAfter {
			return start, startEntry, nil
		}
	case SearchForward:
		if start == height-1 {
			if mode == MatchBefore {
				return start, startEntry, nil
			}
			return 0, nil, ErrReachedChainEnd
		}
	}

	// Binary search for the boundary: the lowest index whose entry does not
	// sort before the target (i.e. find does not return SearchForward).
	lo, hi := uint64(0), height
	for lo < hi {
		mid := lo + (hi-lo)/2
		e, err := read(mid)
		if err != nil {
			return 0, nil, err
		}
		if find(e) == SearchForward {
			lo = mid + 1
		} else {
			hi = mid
		}
	}

	// Every entry sorts before the target, so a forward walk would have run off
	// the end of the chain (the start == height-1 case is handled above)
	if lo == height {
		return 0, nil, ErrReachedChainEnd
	}

	e, err := read(lo)
	if err != nil {
		return 0, nil, err
	}
	switch find(e) {
	case SearchComplete:
		// Entries below lo sort before the target, so lo is the first entry of
		// the run of matches. A walk approaching from below returns the first
		// match; a walk approaching from above returns the last match.
		if start <= lo {
			return lo, e, nil
		}
		for lo+1 < height {
			n, err := read(lo + 1)
			if err != nil {
				return 0, nil, err
			}
			if find(n) != SearchComplete {
				break
			}
			lo, e = lo+1, n
			if start == lo {
				break
			}
		}
		return lo, e, nil

	default: // SearchBackward: lo is the first entry past the target
		if lo == 0 {
			// Every entry sorts after the target, so a backward walk would have
			// run off the start of the chain (the start == 0 && MatchAfter case
			// is handled above)
			if mode == MatchAfter {
				return lo, e, nil
			}
			return 0, nil, ErrReachedChainStart
		}
		switch mode {
		case MatchAfter:
			return lo, e, nil
		case MatchBefore:
			p, err := read(lo - 1)
			if err != nil {
				return 0, nil, err
			}
			return lo - 1, p, nil
		default: // MatchExact
			return 0, nil, ErrTargetDoesNotExist
		}
	}
}

// SearchIndexChainBySource returns a search function that searches an index
// chain for the given source.
func SearchIndexChainBySource(targetSource uint64) IndexChainSearchFunction {
	return func(entry *protocol.IndexEntry) SearchDirection {
		// If the entry is before the target, search forward
		if entry.Source < targetSource {
			return SearchForward
		}

		// If the entry is after the target, search backward
		if entry.Source > targetSource {
			return SearchBackward
		}

		// The target has been found
		return SearchComplete
	}
}

// SearchIndexChainByBlock returns a search function that searches an index
// chain for the given block index.
func SearchIndexChainByBlock(blockIndex uint64) IndexChainSearchFunction {
	return func(entry *protocol.IndexEntry) SearchDirection {
		// If the entry is before the target, search forward
		if entry.BlockIndex < blockIndex {
			return SearchForward
		}

		// If the entry is after the target, search backward
		if entry.BlockIndex > blockIndex {
			return SearchBackward
		}

		// The target has been found
		return SearchComplete
	}
}

// SearchIndexChainByRootIndexIndex returns a search function that searches an
// index chain for the given RootIndexIndex.
func SearchIndexChainByRootIndexIndex(targetRootIndexIndex uint64) IndexChainSearchFunction {
	return func(entry *protocol.IndexEntry) SearchDirection {
		// If the entry is before the target, search forward
		if entry.RootIndexIndex < targetRootIndexIndex {
			return SearchForward
		}

		// If the entry is after the target, search backward
		if entry.RootIndexIndex > targetRootIndexIndex {
			return SearchBackward
		}

		// The target has been found
		return SearchComplete
	}
}

// SearchIndexChainByAnchorBounds returns a search function that searches an
// index chain for an entry with an anchor in the given bounds (inclusive).
func SearchIndexChainByAnchorBounds(lowerBound, upperBound uint64) IndexChainSearchFunction {
	return func(entry *protocol.IndexEntry) SearchDirection {
		// If the entry is before the lower bound, search forward
		if entry.Anchor < lowerBound {
			return SearchForward
		}

		// If the entry is after the upper bound, search backward
		if entry.Anchor > upperBound {
			return SearchBackward
		}

		// The entry is within the bounds
		return SearchComplete
	}
}
