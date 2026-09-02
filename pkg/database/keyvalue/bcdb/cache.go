// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bcdb

import (
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Two caches, because they answer two different questions about two
// different layers.
//
// The HASH/URL cache serves Account(U).Url, which is written once when an
// account is created and then read on every touch of that account
// forever -- putBpt reads it before writing the account's BPT entry, so
// every account a block touches costs one. It is backed by the DYNAMIC
// layer: the permanent layer is read through a window, so a record
// written once at creation and read for the life of the account ages out
// almost immediately and every read after that is a history walk. Under
// load, GetDeep was 32% of all read time.
//
// The SYNTH/ANCHOR CHAIN cache serves the chain records of the synthetic
// ledger and the anchor pool -- elements, their indices, and the mark
// points. Those are backed by the PERMANENT layer, correctly: a chain
// entry is a fact about a position in a log and never moves. They are
// cached because a collection proof replays a run of them, and healing
// asks for overlapping runs.
//
// # Why caching these is safe at all
//
// Both hold records that CANNOT CHANGE. An account's URL is fixed when
// the account is created; a chain element is the I'th entry of an
// append-only log. That is the whole safety argument, and it is why the
// caches have no invalidation: there is no event that could make a
// cached value wrong. A cache over anything mutable here would be a
// correctness bug rather than a slow path, so the classification below
// is deliberately narrow -- it names shapes, and anything it does not
// name is not cached.

// cacheGenSize is how many entries one generation holds.
const cacheGenSize = 200_000

// immutableCache is a two-generation map.
//
// Nothing is ever deleted and nothing is counted per entry: when the hot
// generation fills, it becomes the cold one and a new hot map is
// started, so the oldest entries fall out two generations later, in one
// step, with no eviction bookkeeping and no per-entry cost. A hit in the
// cold generation is promoted, so what is still being read survives the
// swap and what is not, does not.
type immutableCache struct {
	mu   sync.RWMutex
	hot  map[[32]byte][]byte
	cold map[[32]byte][]byte
	max  int

	hits, misses uint64
}

func newImmutableCache(max int) *immutableCache {
	return &immutableCache{hot: make(map[[32]byte][]byte), cold: map[[32]byte][]byte{}, max: max}
}

func (c *immutableCache) get(h [32]byte) ([]byte, bool) {
	c.mu.RLock()
	v, ok := c.hot[h]
	if !ok {
		v, ok = c.cold[h]
	}
	c.mu.RUnlock()

	c.mu.Lock()
	if ok {
		c.hits++
		c.hot[h] = v // Promote: what is still read survives the next swap
	} else {
		c.misses++
	}
	c.mu.Unlock()
	return v, ok
}

func (c *immutableCache) put(h [32]byte, v []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.hot) >= c.max {
		c.cold, c.hot = c.hot, make(map[[32]byte][]byte, c.max/4)
	}
	c.hot[h] = v
}

// drop removes a key from both generations, for a delete.
func (c *immutableCache) drop(h [32]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.hot, h)
	delete(c.cold, h)
}

func (c *immutableCache) stats() (hits, misses uint64, entries int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hits, c.misses, len(c.hot) + len(c.cold)
}

// cacheKind says which cache a key belongs to, if any.
type cacheKind int

const (
	cacheNone  cacheKind = iota
	cacheURL             // Account(U).Url -- dynamic layer
	cacheChain           // synthetic/anchor chain records -- permanent layer
)

// cacheKindOf classifies a key. It names shapes and nothing else: a
// record this does not name is read from the store every time, which is
// the safe direction.
func cacheKindOf(k *record.Key) cacheKind {
	last, prev, trailing := tail(k)

	switch last {
	case "Url":
		// Account(U).Url. Written once at account creation, read by
		// putBpt on every touch of the account.
		if trailing == 0 && prev == "Account" {
			return cacheURL
		}

	case "Element", "ElementIndex", "States":
		// A chain entry: the I'th element, where element H landed, or
		// the mark point covering I. All facts about a position in an
		// append-only log, so none of them move.
		//
		// Restricted to the synthetic ledger and the anchor pool. Every
		// chain element is immutable and would be safe to cache, but
		// these are the ones a collection proof replays and healing
		// re-reads in overlapping runs; admitting every chain in the
		// database would dilute the cache with entries read once.
		if trailing == 1 && isSynthOrAnchorAccount(k) {
			return cacheChain
		}
	}
	return cacheNone
}

// isSynthOrAnchorAccount reports whether an Account key names this
// partition's synthetic ledger or anchor pool.
func isSynthOrAnchorAccount(k *record.Key) bool {
	if k == nil || k.Len() < 2 {
		return false
	}
	if s, ok := k.Get(0).(string); !ok || s != "Account" {
		return false
	}
	u, ok := k.Get(1).(interface{ PathEqual(string) bool })
	if !ok {
		return false
	}
	return u.PathEqual(protocol.Synthetic) || u.PathEqual(protocol.AnchorPool)
}
