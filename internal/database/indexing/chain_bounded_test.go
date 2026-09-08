// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"math/bits"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4263: a receipt for an entry with an OLD anchor must cost a bounded number
// of index-chain reads.
//
// This is the shape of the query that hung. A receipt search starts at the
// NEWEST entry of the root index chain and looks for the entry's anchor, so
// the older the anchor the further back it has to reach. While the search
// walked one entry at a time that was one database read per entry between the
// two, which on a long-running network is hundreds of thousands of reads: the
// v3 API hung past a 180 s client timeout on Kermit for exactly this, while
// the same query without the receipt returned in ~280 ms.
//
// The forty-odd existing receipt tests all build their receipt on a small,
// freshly built chain where the anchor is a few entries back, so none of them
// could see it. This one asserts the cost, not the answer, and it counts
// reads rather than measuring time so a loaded runner cannot make it flaky.
func TestSearchIndexChain_OldestAnchorFromNewestEntryIsBounded(t *testing.T) {
	const n = 10000
	c := makeIndexChain(t, n)

	// Look for the oldest entry, starting from the newest — the receipt case.
	var reads int
	bySource := SearchIndexChainBySource(0)
	counted := func(e *protocol.IndexEntry) SearchDirection {
		reads++
		return bySource(e)
	}

	idx, entry, err := SearchIndexChain(c, uint64(n-1), MatchAfter, counted)
	require.NoError(t, err)
	require.Equal(t, uint64(0), idx, "the oldest entry is at index 0")
	require.Equal(t, uint64(0), entry.Source)

	// A binary search examines about log2(n) entries; the walk it replaced
	// examined all n. The bound is generous — twice log2 plus the handful of
	// boundary probes — and still two orders of magnitude below a walk.
	limit := 2*bits.Len(uint(n)) + 4
	require.LessOrEqual(t, reads, limit,
		"searching back to the oldest anchor examined %d of %d entries; a receipt for an old entry must not scan the chain", reads, n)
}
