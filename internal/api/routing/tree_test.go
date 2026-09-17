// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package routing

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestNewRouteTree spells out the conversion for seven partitions, where the
// prefix table is ragged: one partition holds a 2-bit prefix and six hold 3-bit
// prefixes, so the first gets twice the buckets of each of the others. That
// skew is the thing #4136 is about, and phase 1 carries it over unchanged.
func TestNewRouteTree(t *testing.T) {
	const eighth = BucketCount / 8

	routes := buildSimpleTable([]string{"A", "B", "C", "D", "E", "F", "G"}, 0, 0)
	tree, err := NewRouteTree(&protocol.RoutingTable{Routes: routes})
	require.NoError(t, err)

	require.Equal(t, &RouteTree{
		overrides: map[[32]uint8]string{},
		buckets: &bucketTable{ranges: []bucketRange{
			{Start: 0 * eighth, End: 2 * eighth, Partition: "A"},
			{Start: 2 * eighth, End: 3 * eighth, Partition: "B"},
			{Start: 3 * eighth, End: 4 * eighth, Partition: "C"},
			{Start: 4 * eighth, End: 5 * eighth, Partition: "D"},
			{Start: 5 * eighth, End: 6 * eighth, Partition: "E"},
			{Start: 6 * eighth, End: 7 * eighth, Partition: "F"},
			{Start: 7 * eighth, End: 8 * eighth, Partition: "G"},
		}},
	}, tree)
}

func TestBucketOf(t *testing.T) {
	// The bucket is the leading BucketBits, so the rest of the routing number
	// must not reach it.
	require.Equal(t, uint32(0), BucketOf(0))
	require.Equal(t, uint32(0), BucketOf(1<<(64-BucketBits)-1))
	require.Equal(t, uint32(1), BucketOf(1<<(64-BucketBits)))
	require.Equal(t, uint32(BucketCount-1), BucketOf(^uint64(0)))
}

func FuzzRouteTree_Route(f *testing.F) {
	routes := buildSimpleTable([]string{"A", "B", "C", "D", "E", "F", "G"}, 0, 0)
	tree, err := NewRouteTree(&protocol.RoutingTable{Routes: routes})
	require.NoError(f, err)

	f.Add("foo")
	f.Add("bar/baz")
	f.Fuzz(func(t *testing.T, s string) {
		t.Parallel()
		u, err := url.Parse(s)
		if err != nil {
			t.Skip()
		}

		_, err = tree.Route(u)
		require.NoError(t, err)
	})
}
