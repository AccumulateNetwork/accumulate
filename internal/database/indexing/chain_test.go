// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// searchIndexChainLinear is the original linear-walk implementation of
// SearchIndexChain, kept as the behavioral oracle for the binary search.
func searchIndexChainLinear(chain *database.Chain, index uint64, mode MatchMode, find IndexChainSearchFunction) (uint64, *protocol.IndexEntry, error) {
	entry := new(protocol.IndexEntry)
	err := chain.EntryAs(int64(index), entry)
	if err != nil {
		return 0, nil, fmt.Errorf("entry %d %w", index, err)
	}

	dir := find(entry)
	if dir == SearchComplete {
		return index, entry, nil
	}

	if index == 0 && dir == SearchBackward && mode == MatchAfter {
		return index, entry, nil
	}

	if index == uint64(chain.Height())-1 && dir == SearchForward && mode == MatchBefore {
		return index, entry, nil
	}

	for {
		prevIndex := index
		if dir == SearchForward {
			index++
			if index >= uint64(chain.Height()) {
				return 0, nil, ErrReachedChainEnd
			}
		} else {
			if index == 0 {
				if mode == MatchAfter {
					return index, entry, nil
				}
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

		if dir == dir2 {
			continue
		}

		if dir == SearchBackward {
			prevIndex, index = index, prevIndex
			prevEntry, entry = entry, prevEntry
		}

		switch mode {
		default:
			return 0, nil, ErrTargetDoesNotExist

		case MatchBefore:
			return prevIndex, prevEntry, nil

		case MatchAfter:
			return index, entry, nil
		}
	}
}

// makeIndexChain builds an index chain with n entries. Source increases by 3,
// Anchor by 2, BlockIndex by 1, so targets can fall exactly on, between,
// before, and after entries.
func makeIndexChain(t testing.TB, n int) *database.Chain {
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)

	c2 := batch.Account(protocol.AccountUrl("test")).MainChain().Index()
	for i := 0; i < n; i++ {
		entry := &protocol.IndexEntry{
			Source:     uint64(3 * i),
			Anchor:     uint64(2 * i),
			BlockIndex: uint64(i + 1),
		}
		b, err := entry.MarshalBinary()
		require.NoError(t, err)
		require.NoError(t, c2.Inner().AddEntry(b, false))
	}

	c, err := c2.Get()
	require.NoError(t, err)
	return c
}

func requireSameSearchResult(t *testing.T, c *database.Chain, start uint64, mode MatchMode, find IndexChainSearchFunction, label string) {
	t.Helper()
	wantIdx, wantEntry, wantErr := searchIndexChainLinear(c, start, mode, find)
	gotIdx, gotEntry, gotErr := SearchIndexChain(c, start, mode, find)

	if wantErr != nil {
		require.Error(t, gotErr, label)
		require.Equal(t, wantErr.Error(), gotErr.Error(), label)
		return
	}
	require.NoError(t, gotErr, label)
	require.Equal(t, wantIdx, gotIdx, label)
	require.True(t, wantEntry.Equal(gotEntry), label)
}

func TestSearchIndexChainEquivalence(t *testing.T) {
	for _, n := range []int{1, 2, 3, 10, 100, 300} {
		c := makeIndexChain(t, n)
		height := uint64(c.Height())
		require.Equal(t, uint64(n), height)

		starts := map[uint64]bool{0: true, height - 1: true, height / 2: true}
		if height > 3 {
			starts[1] = true
			starts[height-2] = true
		}

		maxSource := uint64(3 * (n - 1))
		for start := range starts {
			for _, mode := range []MatchMode{MatchExact, MatchBefore, MatchAfter} {
				// Every source value on, between, before, and past entries
				for target := uint64(0); target <= maxSource+3; target++ {
					label := fmt.Sprintf("n=%d start=%d mode=%d source=%d", n, start, mode, target)
					requireSameSearchResult(t, c, start, mode, SearchIndexChainBySource(target), label)
				}
				// Block index targets, including out of range on both sides
				for target := uint64(0); target <= uint64(n)+2; target++ {
					label := fmt.Sprintf("n=%d start=%d mode=%d block=%d", n, start, mode, target)
					requireSameSearchResult(t, c, start, mode, SearchIndexChainByBlock(target), label)
				}
				// Anchor bounds, including runs of multiple matching entries
				for lower := uint64(0); lower <= uint64(2*n)+2; lower += 3 {
					for _, width := range []uint64{0, 1, 5, uint64(2 * n)} {
						label := fmt.Sprintf("n=%d start=%d mode=%d anchor=[%d,%d]", n, start, mode, lower, lower+width)
						requireSameSearchResult(t, c, start, mode, SearchIndexChainByAnchorBounds(lower, lower+width), label)
					}
				}
			}
		}
	}
}

func BenchmarkSearchIndexChain(b *testing.B) {
	const n = 100_000
	c := makeIndexChain(b, n)
	// Worst case for the linear walk: search from the newest entry for the
	// oldest source, i.e. a receipt for an old chain entry.
	find := SearchIndexChainBySource(0)

	b.Run("binary", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _, err := SearchIndexChain(c, n-1, MatchAfter, find)
			require.NoError(b, err)
		}
	})
	b.Run("linear", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _, err := searchIndexChainLinear(c, n-1, MatchAfter, find)
			require.NoError(b, err)
		}
	})
}
