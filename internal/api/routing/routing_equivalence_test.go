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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func partitionNames(n int) []string {
	bvns := make([]string, n)
	for i := range bvns {
		bvns[i] = fmt.Sprintf("BVN%d", i)
	}
	return bvns
}

// TestBucketRoutingMatchesPrefixRouting is the acceptance test for #4136 phase
// 1: buckets are a change of representation, so every account must route
// exactly where the prefix table sent it. The comparison is against the prefix
// walk itself (prefix_reference_test.go), not against a description of it.
//
// Routing numbers are exercised directly rather than through URLs so that the
// boundaries can be hit deliberately: the bucket a prefix ends on, and the one
// either side of it, are where a shift that is off by one shows up, and random
// hashes reach them only by luck.
func TestBucketRoutingMatchesPrefixRouting(t *testing.T) {
	for n := 1; n <= 16; n++ {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			routes := BuildSimpleTable(partitionNames(n))

			prefix, err := newPrefixTree(routes)
			require.NoError(t, err)
			tree, err := NewRouteTree(&protocol.RoutingTable{Routes: routes})
			require.NoError(t, err)

			check := func(rn uint64) {
				t.Helper()
				want, err := prefix.route(rn, 0)
				require.NoError(t, err)
				got, err := tree.RouteNr(rn)
				require.NoError(t, err)
				if want != got {
					t.Fatalf("routing number %#016x: prefix table says %s, buckets say %s", rn, want, got)
				}
			}

			// Every bucket boundary, and the numbers either side of it.
			for _, r := range routes {
				width := uint64(1) << (BucketBits - r.Length)
				for _, bucket := range []uint64{r.Value * width, (r.Value + 1) * width} {
					for _, delta := range []int64{-1, 0, 1} {
						b := int64(bucket) + delta
						if b < 0 || b >= BucketCount {
							continue
						}
						// The lowest and highest routing number in the bucket.
						check(uint64(b) << (64 - BucketBits))
						check(uint64(b)<<(64-BucketBits) | (1<<(64-BucketBits) - 1))
					}
				}
			}

			// And a large random sample.
			rng := rand.New(rand.NewSource(int64(n)))
			for i := 0; i < 200_000; i++ {
				check(rng.Uint64())
			}
		})
	}
}

// TestBucketRoutingMatchesPrefixRoutingForADIs repeats the acceptance test over
// real account URLs, so that the hashes under test are the ones the network
// actually routes rather than numbers chosen by a generator.
func TestBucketRoutingMatchesPrefixRoutingForADIs(t *testing.T) {
	accounts := make([]*url.URL, 0, 20_000)
	for i := 0; i < cap(accounts); i++ {
		accounts = append(accounts, url.MustParse(fmt.Sprintf("acc://adi%d.acme", i)))
	}

	for n := 1; n <= 8; n++ {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			routes := BuildSimpleTable(partitionNames(n))

			prefix, err := newPrefixTree(routes)
			require.NoError(t, err)
			tree, err := NewRouteTree(&protocol.RoutingTable{Routes: routes})
			require.NoError(t, err)

			for _, u := range accounts {
				want, err := prefix.route(u.Routing(), 0)
				require.NoError(t, err)
				got, err := tree.Route(u)
				require.NoError(t, err)
				if want != got {
					t.Fatalf("%v: prefix table says %s, buckets say %s", u, want, got)
				}
			}
		})
	}
}

// TestBucketRoutingPreservesTheSkew records what phase 1 deliberately does NOT
// fix. Three BVNs still get 50/25/25, because the assignment is unchanged and
// only its representation moved. A phase 1 that evened this out would have
// moved accounts, and would be wrong (#4136).
func TestBucketRoutingPreservesTheSkew(t *testing.T) {
	const sample = 200_000

	expect := map[int]map[string]float64{
		2: {"BVN0": 0.50, "BVN1": 0.50},
		3: {"BVN0": 0.50, "BVN1": 0.25, "BVN2": 0.25},
		4: {"BVN0": 0.25, "BVN1": 0.25, "BVN2": 0.25, "BVN3": 0.25},
		5: {"BVN0": 0.25, "BVN1": 0.25, "BVN2": 0.25, "BVN3": 0.125, "BVN4": 0.125},
	}

	for n, want := range expect {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			tree, err := NewRouteTree(&protocol.RoutingTable{Routes: BuildSimpleTable(partitionNames(n))})
			require.NoError(t, err)

			count := map[string]int{}
			rng := rand.New(rand.NewSource(1))
			for i := 0; i < sample; i++ {
				p, err := tree.RouteNr(rng.Uint64())
				require.NoError(t, err)
				count[p]++
			}

			for part, share := range want {
				got := float64(count[part]) / sample
				require.InDelta(t, share, got, 0.01, "%s share", part)
			}
		})
	}
}

// TestRouteOverridesWin checks that a per-account override still beats the
// bucket assignment, which is what pins the DN and any busy ADI to a chosen
// partition.
func TestRouteOverridesWin(t *testing.T) {
	account := url.MustParse("acc://pinned.acme")

	table := &protocol.RoutingTable{Routes: BuildSimpleTable(partitionNames(4))}
	tree, err := NewRouteTree(table)
	require.NoError(t, err)
	natural, err := tree.Route(account)
	require.NoError(t, err)

	var elsewhere string
	for _, p := range partitionNames(4) {
		if p != natural {
			elsewhere = p
			break
		}
	}
	require.NotEmpty(t, elsewhere)

	table.AddOverride(account, elsewhere)
	tree, err = NewRouteTree(table)
	require.NoError(t, err)

	got, err := tree.Route(account)
	require.NoError(t, err)
	require.Equal(t, elsewhere, got, "the override must win over the bucket assignment")
}

// TestNewRouteTreeRejectsBadTables checks that a table which does not assign
// every bucket to exactly one partition is refused when it is loaded, rather
// than routing fine until an account happens to land in the hole.
func TestNewRouteTreeRejectsBadTables(t *testing.T) {
	cases := []struct {
		name   string
		routes []protocol.Route
		expect string
	}{
		{"no routes", nil, "no routes"},
		{"a gap", []protocol.Route{
			{Length: 2, Value: 0b00, Partition: "A"},
			{Length: 2, Value: 0b01, Partition: "B"},
			{Length: 2, Value: 0b11, Partition: "C"},
		}, "no partition"},
		{"an overlap", []protocol.Route{
			{Length: 1, Value: 0b0, Partition: "A"},
			{Length: 2, Value: 0b01, Partition: "B"},
			{Length: 1, Value: 0b1, Partition: "C"},
		}, "assigned twice"},
		{"a prefix longer than a bucket", []protocol.Route{
			{Length: BucketBits + 1, Value: 0, Partition: "A"},
		}, "exceeds"},
		{"a value too big for its prefix", []protocol.Route{
			{Length: 1, Value: 0b10, Partition: "A"},
		}, "does not fit"},
		{"a route with no partition", []protocol.Route{
			{Length: 0, Value: 0, Partition: ""},
		}, "no partition"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewRouteTree(&protocol.RoutingTable{Routes: c.routes})
			require.Error(t, err)
			require.Contains(t, err.Error(), c.expect)
		})
	}
}

// TestNewRouteTreeDoesNotModifyTheTable guards a side effect the prefix tree
// had: it sorted the caller's route slice in place, and that slice belongs to
// the network's global values, shared with every other reader of them.
func TestNewRouteTreeDoesNotModifyTheTable(t *testing.T) {
	table := &protocol.RoutingTable{Routes: BuildSimpleTable(partitionNames(5))}
	before := append([]protocol.Route(nil), table.Routes...)

	_, err := NewRouteTree(table)
	require.NoError(t, err)
	require.Equal(t, before, table.Routes)
}
