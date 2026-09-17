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

// This is the prefix tree that routed accounts before #4136 replaced it with
// buckets, kept verbatim and only for tests. Bucket routing claims to send
// every account exactly where this sent it; routing_equivalence_test.go checks
// that claim against this code rather than against a description of it.
//
// It is dead weight the day someone is confident enough to delete it. Until
// then it is the only witness to what the routing used to do.

type prefixTreeNode interface {
	route(rn uint64, pos uint16) (string, error)
}

type prefixTreeBranch struct {
	bits     uint16
	children []prefixTreeNode
}

type prefixTreeLeaf string

func newPrefixTree(routes []protocol.Route) (prefixTreeNode, error) {
	routes = append([]protocol.Route(nil), routes...)
	sort.Slice(routes, func(i, j int) bool {
		r, s := routes[i], routes[j]
		v, u := r.Value<<(64-r.Length), s.Value<<(64-s.Length)
		return v < u
	})
	return buildPrefixTree(routes, 0)
}

func buildPrefixTree(routes []protocol.Route, depth uint64) (prefixTreeNode, error) {
	if len(routes) == 1 {
		r := routes[0]
		if r.Length != depth {
			return nil, errors.InternalError.WithFormat("expected offset %d, got %d", depth, r.Length)
		}
		return prefixTreeLeaf(r.Partition), nil
	}

	// Get the minimum offset
	offset := routes[0].Length
	for _, r := range routes[1:] {
		if r.Length < offset {
			offset = r.Length
		}
	}

	var tree prefixTreeBranch
	var err error
	tree.bits = uint16(offset - depth)
	tree.children = make([]prefixTreeNode, 1<<tree.bits)
	mask := uint64(1<<tree.bits - 1)
	for i := range tree.children {
		n := sort.Search(len(routes), func(j int) bool {
			r := routes[j]
			v := r.Value >> (r.Length - offset)
			return v&mask > uint64(i)
		})
		if n == 0 {
			return nil, errors.InternalError.WithFormat("expected values with %b at %d:%d, found none", i, offset, depth)
		}
		tree.children[i], err = buildPrefixTree(routes[:n], offset)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		routes = routes[n:]
	}
	return tree, err
}

func (b prefixTreeBranch) route(rn uint64, pos uint16) (string, error) {
	npos := pos + b.bits
	i := (rn >> uint64(64-npos)) & (1<<b.bits - 1)
	if b.children[i] == nil {
		return "", errors.InternalError.WithFormat("invalid routing table: no entry for %d at %d.%d", i, pos, b.bits)
	}

	return b.children[i].route(rn, npos)
}

func (b prefixTreeLeaf) route(_ uint64, _ uint16) (string, error) {
	return string(b), nil
}
