// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package routing

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A RouteTree answers which partition an account belongs to. Per-account
// overrides are checked first; everything else routes by bucket (see
// bucket.go).
type RouteTree struct {
	overrides map[[32]byte]string
	buckets   *bucketTable
}

// NewRouteTree builds a route tree from a routing table. It returns an error if
// the table does not assign every bucket to exactly one partition.
//
// The table is not modified. An earlier version sorted table.Routes in place,
// which reordered the caller's slice -- and that slice belongs to the network's
// global values, shared with every other reader of them.
func NewRouteTree(table *protocol.RoutingTable) (*RouteTree, error) {
	tree := new(RouteTree)

	tree.overrides = make(map[[32]byte]string, len(table.Overrides))
	for _, o := range table.Overrides {
		tree.overrides[o.Account.IdentityAccountID32()] = o.Partition
	}

	var err error
	tree.buckets, err = newBucketTable(table.Routes)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return tree, nil
}

func (r *RouteTree) Route(u *url.URL) (string, error) {
	s, ok := r.overrides[u.IdentityAccountID32()]
	if ok {
		return s, nil
	}

	return r.RouteNr(u.Routing())
}

func (r *RouteTree) RouteNr(n uint64) (string, error) {
	return r.buckets.route(BucketOf(n))
}
