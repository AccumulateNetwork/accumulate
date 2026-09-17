// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package routing

import (
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// An account's routing number is the first 8 bytes of its identity account ID
// (URL.Routing()). Its bucket is the leading BucketBits of that number, so
// every account falls in exactly one of BucketCount buckets and a partition is
// assigned whole buckets rather than a bit prefix.
//
// Bit prefixes can only divide the space into powers of two, so three
// partitions are stuck with 50/25/25 and one BVN carries twice the accounts of
// each of the others (#4136). Buckets divide it as finely as BucketCount
// allows.
//
// Buckets are the leading bits, not the whole number modulo a count, and that
// is what makes this change free: a bucket nests inside a prefix, so every
// prefix route is exactly a contiguous run of buckets. Converting a prefix
// table to buckets is a change of representation that moves no account. What it
// buys is the ability to express an assignment prefixes cannot -- an even one,
// and an interleaved one that leaves the leading bits usable for execution
// sharding -- which is the next phase, and needs account migration.
const (
	// BucketBits is how many leading bits of the routing number name a bucket.
	BucketBits = 20

	// BucketCount is how many buckets the routing space divides into.
	BucketCount = 1 << BucketBits
)

// BucketOf returns the bucket a routing number falls in.
func BucketOf(routingNr uint64) uint32 {
	return uint32(routingNr >> (64 - BucketBits))
}

// A bucketRange assigns the buckets [Start, End) to a partition.
type bucketRange struct {
	Start, End uint32
	Partition  string
}

// A bucketTable says which partition every bucket routes to. Its ranges are
// sorted, non-overlapping, and cover [0, BucketCount) with no gap, which
// newBucketTable checks and route therefore does not have to.
type bucketTable struct {
	ranges []bucketRange
}

// newBucketTable converts prefix routes to bucket ranges.
//
// A route of length L and value V covers the routing numbers whose leading L
// bits are V. Because a bucket is the leading BucketBits, that is exactly the
// buckets [V << (BucketBits-L), (V+1) << (BucketBits-L)) -- and for L of 0, the
// whole space. Routing by bucket therefore answers exactly what the prefix walk
// answered, which routing_equivalence_test.go checks against the prefix walk
// itself over every table shape up to 16 partitions.
func newBucketTable(routes []protocol.Route) (*bucketTable, error) {
	if len(routes) == 0 {
		return nil, errors.InternalError.With("invalid routing table: no routes")
	}

	ranges := make([]bucketRange, 0, len(routes))
	for _, r := range routes {
		// A prefix longer than a bucket would split one bucket between two
		// partitions, which buckets cannot express. BucketBits of 20 allows a
		// million partitions, so this is a malformed table, not a limit anyone
		// reaches.
		if r.Length > BucketBits {
			return nil, errors.InternalError.WithFormat(
				"invalid routing table: prefix length %d exceeds the %d bits of a bucket", r.Length, BucketBits)
		}
		if r.Value >= 1<<r.Length {
			return nil, errors.InternalError.WithFormat(
				"invalid routing table: value %d does not fit in a %d bit prefix", r.Value, r.Length)
		}
		if r.Partition == "" {
			return nil, errors.InternalError.With("invalid routing table: route with no partition")
		}

		width := uint32(1) << (BucketBits - r.Length)
		start := uint32(r.Value) * width
		ranges = append(ranges, bucketRange{Start: start, End: start + width, Partition: r.Partition})
	}

	sort.Slice(ranges, func(i, j int) bool { return ranges[i].Start < ranges[j].Start })

	// Every bucket must route somewhere, and to only one partition. The prefix
	// tree reported a gap as "no entry for N at D.B" only when an account
	// happened to land in it; checking coverage up front turns that into a
	// rejected table.
	var next uint32
	for _, b := range ranges {
		if b.Start != next {
			if b.Start < next {
				return nil, errors.InternalError.WithFormat(
					"invalid routing table: buckets [%d, %d) are assigned twice", b.Start, next)
			}
			return nil, errors.InternalError.WithFormat(
				"invalid routing table: buckets [%d, %d) are assigned to no partition", next, b.Start)
		}
		next = b.End
	}
	if next != BucketCount {
		return nil, errors.InternalError.WithFormat(
			"invalid routing table: buckets [%d, %d) are assigned to no partition", next, uint32(BucketCount))
	}

	return &bucketTable{ranges: ranges}, nil
}

// route returns the partition a bucket is assigned to.
func (t *bucketTable) route(bucket uint32) (string, error) {
	i := sort.Search(len(t.ranges), func(i int) bool { return t.ranges[i].End > bucket })
	if i == len(t.ranges) || t.ranges[i].Start > bucket {
		// newBucketTable rejects a table that leaves a bucket unassigned, so
		// this is unreachable; it is here so that a future assignment format
		// cannot turn a coverage bug into a silent misroute.
		return "", errors.InternalError.WithFormat("invalid routing table: no entry for bucket %d", bucket)
	}
	return t.ranges[i].Partition, nil
}
