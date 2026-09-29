// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package synthcache

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A Store answers for what this partition produced and the cache no longer
// holds, from the partition's own chains (healing spec, "The cache"; #4464).
// The cache holds what is in play; a destination can fall further behind
// than that — past the horizon, or across a restart whose seed does not reach
// back to where it stopped — and what it still lacks is on the producer's
// synthetic and anchor sequence chains, where nothing else can supply it.
type Store interface {
	// SyntheticBlock rebuilds the block that produced synthetic number of
	// the stream to dst, dispatched under the Directory anchor that
	// receipted it. NotFound when the number is not produced or the store
	// does not hold the block, NotReady when the Directory has not
	// receipted it yet, and ErrNotExecuted when this node did not execute
	// the block.
	SyntheticBlock(dst *url.URL, number uint64) (*Block, error)

	// ProducedAnchor reads produced anchor number from the anchor sequence
	// chain, with the block it anchors. NotFound when it is not held.
	ProducedAnchor(number uint64) (*protocol.Transaction, uint64, error)
}

// ErrNotExecuted is a Store's answer for a block this node did not execute:
// a join carried it past the block, and the nodes that executed it answer
// for it.
var ErrNotExecuted = errNotExecuted{}

type errNotExecuted struct{}

func (errNotExecuted) Error() string { return "this node did not execute the block" }

type storeHolder struct{ Store }

// SetStore wires the store the cache's readers fall back to (the executor).
func (c *Cache) SetStore(s Store) { c.store.Store(&storeHolder{s}) }

// Store is the store wired by SetStore, or nil.
func (c *Cache) Store() Store {
	h := c.store.Load()
	if h == nil {
		return nil
	}
	return h.Store
}
