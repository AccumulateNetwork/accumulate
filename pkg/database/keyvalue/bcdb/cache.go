// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bcdb

import (
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// A read cache for the records the executor reads over and over.
//
// # Why
//
// Reads of the permanent layer go through the store's segment index, and a
// miss in the newest segment walks older ones, testing each segment's bloom
// filter — and the filters are read from disk to be tested. Profiled under
// load in run 20260902T041031Z: syscall.Syscall6 was 18.45% of CPU, pread was
// 16.5% of the total, and 71.8% of those preads were segment.bloomTest
// fetching filters. Actual value reads were under 9% of them. The cost of a
// read is finding the record, not returning it.
//
// The records that pay this most are the ones the executor reaches for on
// every block: an account's URL, and the synthetic transactions and anchors it
// is delivering. They are also, all three, WRITE-ONCE — an account's URL never
// changes, and a message or a transaction is named by the hash of its own
// content — so a cached value cannot go stale by being rewritten.
//
// # Shape
//
// Two generations. Lookups try the hot map, then the cold one; a hit in cold
// is promoted into hot, so anything still being used survives. When hot fills,
// it becomes cold and a new hot is started — so entries leave by being unused
// for a whole generation, and nothing is evicted one at a time.
//
// This bounds the cache at 2N entries and makes eviction O(1) amortized: the
// cost of a generation change is dropping a map reference, not scanning
// anything, and no entry is ever removed individually. An LRU would order
// every entry on every hit, against a workload where hits are the common case.
type recordCache struct {
	mu    sync.Mutex
	limit int
	hot   map[[32]byte][]byte
	cold  map[[32]byte][]byte

	hits, misses, promotions, generations uint64
}

// DefaultCacheEntries is how many entries the hot generation holds before it
// becomes the cold one. The cache holds up to twice this.
//
// 20,000 covers the working set the executor touches across a run of blocks —
// the accounts under load, and the synthetics and anchors in flight — without
// the map itself becoming the memory problem. Values are small: a URL record,
// a message, an anchor.
const DefaultCacheEntries = 20_000

func newRecordCache(limit int) *recordCache {
	if limit <= 0 {
		limit = DefaultCacheEntries
	}
	return &recordCache{
		limit: limit,
		hot:   make(map[[32]byte][]byte, limit/8),
		cold:  map[[32]byte][]byte{},
	}
}

// get returns a cached value. A hit in the cold generation is promoted, which
// is what keeps a record that is still in use from aging out.
func (c *recordCache) get(h [32]byte) ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if v, ok := c.hot[h]; ok {
		c.hits++
		return v, true
	}
	if v, ok := c.cold[h]; ok {
		c.hits++
		c.promotions++
		c.putLocked(h, v)
		return v, true
	}
	c.misses++
	return nil, false
}

// put records a value. Only write-once records may be cached; see cacheable.
func (c *recordCache) put(h [32]byte, value []byte) {
	if c == nil || len(value) == 0 {
		return // A zero-length value is a deletion, and is not cached
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.putLocked(h, value)
}

func (c *recordCache) putLocked(h [32]byte, value []byte) {
	c.hot[h] = value
	if len(c.hot) < c.limit {
		return
	}
	// The generation is full: it becomes the cold one and a new hot starts.
	// Whatever was in cold and never got promoted is dropped with it.
	c.cold = c.hot
	c.hot = make(map[[32]byte][]byte, c.limit/8)
	c.generations++
}

// stats reports the counters for stats.json.
func (c *recordCache) stats() (hits, misses, promotions, generations uint64, entries int) {
	if c == nil {
		return 0, 0, 0, 0, 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits, c.misses, c.promotions, c.generations, len(c.hot) + len(c.cold)
}

// cacheable reports whether a key names a record the cache may hold.
//
// Narrower than isWriteOnce, and by hand rather than by reusing it: this names
// the three kinds asked for, each of which cannot change under its own name:
//
//   - Account(U).Url, the URL an account is keyed by
//   - Message(H).Main and Transaction(H).Main, which the executor reads to
//     deliver synthetic transactions, and which are named by the hash of their
//     own content
//   - the anchor chains' elements and mark points, which are positions in a
//     log and do not move
//
// # Why nothing invalidates
//
// Nothing is removed from this cache by key, and a write does not touch it. An
// entry leaves only by sitting unread in the cold generation through a swap.
//
// That needs no defending, because the records here cannot change. An anchor
// that changed, a synthetic transaction that changed, an account URL that
// changed under the same key -- any of those is a broken protocol, and a stale
// cache entry would be the smallest of the consequences.
//
// The guard is this list, and code review of it. The bar for adding a shape is
// that it cannot change under its own name.
func cacheable(k *record.Key) bool {
	last, prev, trailing := tail(k)
	switch last {
	case "Url":
		return trailing == 0 && prev == "Account"

	case "Main":
		return trailing == 0 && (prev == "Message" || prev == "Transaction")

	case "Element", "States":
		// A chain's I'th entry, and the mark point covering I. Restricted to
		// the anchor and sequence chains, which is what the executor reads
		// while delivering: the general chain case is left alone.
		return trailing == 1 && isAnchorChain(k)
	}
	return false
}

// isAnchorChain reports whether the key names an anchor or anchor-sequence
// chain, by looking for the chain name in the key rather than at a fixed
// position -- AnchorChain carries a partition name after it
// (AnchorChain.bvn1.Root.States), and AnchorSequenceChain does not.
func isAnchorChain(k *record.Key) bool {
	for i := 0; i < k.Len(); i++ {
		if s, ok := k.Get(i).(string); ok {
			switch s {
			case "AnchorChain", "AnchorSequenceChain":
				return true
			}
		}
	}
	return false
}
