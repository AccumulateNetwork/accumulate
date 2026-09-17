// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package routing

import (
	"math/bits"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// BuildEvenTable divides the bucket space into equal contiguous runs, one per
// partition, so that every partition carries the same share of accounts to
// within a single bucket.
//
// BuildSimpleTable, which this replaces for new networks, splits by bit prefix
// and so can only divide into powers of two: three partitions get 50/25/25 and
// the first carries twice the accounts of each of the others (#4136). Dividing
// the buckets instead removes that, and it needs no change to the routing
// table's format. A run of buckets is not usually a single prefix, but it is
// always a small union of them -- [0, 349525) is a 1/4 block, then a 1/16
// block, then a 1/64 block, and so on down -- so an even split costs at most
// two routes per bit of granularity per partition, and three partitions come
// out as about fifty routes rather than three.
//
// This changes where accounts live, so it is for networks being created. An
// existing network's assignment is its own, stored on chain at
// acc://dn.acme/routing, and changing it moves accounts, which needs the
// migration #4136 defers to a later phase.
//
// What this does NOT do is interleave. Each partition still holds one
// contiguous run, so the leading bits of an account's hash still say which
// partition it is on, and execution sharding on those same bits still sees only
// part of the space. Fixing that is the interleaved assignment, which cannot be
// written as prefixes at all and so waits for the table format to change.
func BuildEvenTable(bvns []string) []protocol.Route {
	if len(bvns) == 0 {
		return nil
	}

	routes := make([]protocol.Route, 0, len(bvns)*2*BucketBits)
	n := uint64(len(bvns))
	for i, partition := range bvns {
		lo := uint64(i) * BucketCount / n
		hi := uint64(i+1) * BucketCount / n
		routes = appendBucketRun(routes, lo, hi, partition)
	}
	return routes
}

// appendBucketRun writes the buckets [lo, hi) as routes.
//
// A route covers a run of buckets whose length is a power of two and which
// starts at a multiple of that length, so the run is emitted greedily: at each
// step take the largest such block that starts at lo and does not overrun hi.
// That is the fewest routes the run can be written in.
func appendBucketRun(routes []protocol.Route, lo, hi uint64, partition string) []protocol.Route {
	for lo < hi {
		// The largest block that may start at lo is fixed by lo's alignment --
		// the lowest set bit -- except at zero, which any block may start at.
		size := uint64(BucketCount)
		if lo != 0 {
			size = lo & -lo
		}
		for size > hi-lo {
			size >>= 1
		}

		width := uint64(bits.TrailingZeros64(size))
		routes = append(routes, protocol.Route{
			Length:    BucketBits - width,
			Value:     lo >> width,
			Partition: partition,
		})
		lo += size
	}
	return routes
}
