// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var ErrReachedChainEnd = errors.New(errors.StatusNotFound, "reached the end of the chain")
var ErrReachedChainStart = errors.New(errors.StatusNotFound, "reached the start of the chain")
var ErrTargetDoesNotExist = errors.New(errors.StatusNotFound, "target does not exist")

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

// SearchIndexChain searches along an index chain using the given search
// function. The search starts from the given index and proceeds forwards or
// backwards along the chain depending on the result of the search function.
func SearchIndexChain(chain *database.Chain, index uint64, mode MatchMode, find IndexChainSearchFunction) (uint64, *protocol.IndexEntry, error) {
	entry := new(protocol.IndexEntry)
	err := chain.EntryAs(int64(index), entry)
	if err != nil {
		return 0, nil, fmt.Errorf("entry %d %w", index, err)
	}

	dir := find(entry)
	if dir == SearchComplete {
		return index, entry, nil
	}

	// If the entry is the first and is after the target and the mode is after,
	// return it
	if index == 0 && dir == SearchBackward && mode == MatchAfter {
		return index, entry, nil
	}

	// If the entry is the last and is before the target and the mode is before,
	// return it
	if index == uint64(chain.Height())-1 && dir == SearchForward && mode == MatchBefore {
		return index, entry, nil
	}

	for {
		// TODO Add a guard to prevent scanning the entire chain?
		prevIndex := index
		if dir == SearchForward {
			index++
			if index >= uint64(chain.Height()) {
				return 0, nil, ErrReachedChainEnd
			}
		} else {
			if index == 0 {
				return 0, nil, ErrReachedChainStart
			}
			index--
		}

		prevEntry := entry
		entry = new(protocol.IndexEntry)
		err := chain.EntryAs(int64(index), entry)
		if err != nil {
			return 0, nil, fmt.Errorf("entry %d %w", index, err)
		}

		dir2 := find(entry)
		if dir2 == 0 {
			return index, entry, nil
		}

		// If the direction is unchanged, continue searching
		if dir == dir2 {
			continue
		}

		if dir == SearchBackward {
			prevIndex, index = index, prevIndex
			prevEntry, entry = entry, prevEntry
		}

		// If the starting and current entries are on either side of the target,
		// the target does not exist
		switch mode {
		default: // SearchExact
			return 0, nil, ErrTargetDoesNotExist

		case MatchBefore:
			return prevIndex, prevEntry, nil

		case MatchAfter:
			return index, entry, nil
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
