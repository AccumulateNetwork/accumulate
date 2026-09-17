// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package routing

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestBuildEvenTableIsEven is the point of the thing: every partition holds the
// same number of buckets, to within one. Three partitions is the case that
// matters, because that is where prefixes give 50/25/25.
func TestBuildEvenTableIsEven(t *testing.T) {
	for n := 1; n <= 16; n++ {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			routes := BuildEvenTable(partitionNames(n))

			// NewRouteTree refuses a table that leaves a bucket unassigned or
			// assigns one twice, so this also checks the decomposition covers
			// the space exactly.
			tree, err := NewRouteTree(&protocol.RoutingTable{Routes: routes})
			require.NoError(t, err)

			buckets := map[string]uint64{}
			for _, r := range routes {
				buckets[r.Partition] += uint64(1) << (BucketBits - r.Length)
			}
			require.Len(t, buckets, n)

			var total uint64
			for _, held := range buckets {
				total += held
			}
			require.Equal(t, uint64(BucketCount), total)

			ideal := uint64(BucketCount) / uint64(n)
			for partition, held := range buckets {
				require.InDelta(t, ideal, held, 1, "%s holds %d buckets, ideal %d", partition, held, ideal)
			}

			// And the same through the router, over routing numbers rather
			// than by counting the table.
			count := map[string]int{}
			rng := rand.New(rand.NewSource(int64(n)))
			const sample = 200_000
			for i := 0; i < sample; i++ {
				p, err := tree.RouteNr(rng.Uint64())
				require.NoError(t, err)
				count[p]++
			}
			for partition, got := range count {
				require.InDelta(t, 1.0/float64(n), float64(got)/sample, 0.01, "%s share", partition)
			}
		})
	}
}

// TestBuildEvenTableFixesTheSkew states the before and after for three
// partitions directly, because that number is the reason #4136 exists.
func TestBuildEvenTableFixesTheSkew(t *testing.T) {
	share := func(routes []protocol.Route) map[string]float64 {
		out := map[string]float64{}
		for _, r := range routes {
			out[r.Partition] += float64(uint64(1)<<(BucketBits-r.Length)) / BucketCount
		}
		return out
	}

	prefixes := share(BuildSimpleTable(partitionNames(3)))
	require.InDelta(t, 0.50, prefixes["BVN0"], 1e-9)
	require.InDelta(t, 0.25, prefixes["BVN1"], 1e-9)
	require.InDelta(t, 0.25, prefixes["BVN2"], 1e-9)

	even := share(BuildEvenTable(partitionNames(3)))
	for _, p := range partitionNames(3) {
		require.InDelta(t, 1.0/3.0, even[p], 1e-5, "%s share", p)
	}
}

// TestBuildEvenTableStaysSmall checks that expressing a run of buckets as
// prefixes does not blow the table up. The bound is two routes per bit of
// granularity per partition; the real figure should be well under it.
func TestBuildEvenTableStaysSmall(t *testing.T) {
	for n := 1; n <= 16; n++ {
		routes := BuildEvenTable(partitionNames(n))
		require.LessOrEqual(t, len(routes), n*2*BucketBits,
			"%d partitions produced %d routes", n, len(routes))
	}

	// Powers of two divide exactly, so they must still cost one route each.
	for _, n := range []int{1, 2, 4, 8, 16} {
		require.Len(t, BuildEvenTable(partitionNames(n)), n, "%d partitions", n)
	}
}

// TestBuildEvenTableIsDeterministic guards the property consensus needs: every
// node building the table for the same partitions must get the same table.
func TestBuildEvenTableIsDeterministic(t *testing.T) {
	for n := 1; n <= 16; n++ {
		first := BuildEvenTable(partitionNames(n))
		for i := 0; i < 5; i++ {
			require.Equal(t, first, BuildEvenTable(partitionNames(n)))
		}
	}
}
