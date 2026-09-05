// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package synthcache is the producer's synthetic/anchor cache (healing spec,
// "The cache"): what this partition has produced that a destination may
// still ask for, held in memory and read by dispatch and by healing. It is
// filled at production, one write on a path the block already takes, and it
// holds per block what a proof is built from, so a package or a bundle and
// its proof are built from the cache alone. Any read of the historical
// record to build one is a failure, and a miss here is counted as one.
package synthcache

import (
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// DefaultHorizon is how many blocks a produced entry is kept: about an hour
// at one block a second, the sanity horizon the rest of the specification
// uses. Entries a destination is known to have delivered can go sooner;
// until that signal exists this bound is what clears the cache.
const DefaultHorizon = 3600

// An Entry is one produced synthetic message: the sequenced message, the
// transaction it belongs to when it has one, and where it sits on the
// producer's synthetic main chain.
type Entry struct {
	Stream    *url.URL // destination partition
	Number    uint64   // sequence number on the stream
	Index     int64    // position on the producer's synthetic main chain
	Block     uint64   // the block that produced it
	Hash      [32]byte // Seq.Hash()
	Seq       *messaging.SequencedMessage
	Companion messaging.Message
}

// A Block is what a proof for the block's entries is built from: the
// synthetic main chain's segment for the block (state before its first
// element, and its elements), and the receipt from the synthetic chain's
// anchor in the root chain to the root chain's anchor at block close. The
// Directory's receipt for the block arrives later, in a DirectoryAnchor,
// and is combined at dispatch.
type Block struct {
	Index       uint64
	Segment     *merkle.Segment
	RootReceipt *merkle.Receipt
	Entries     []*Entry // in synthetic chain order

	// Set when the block's synthetics were dispatched: the Directory anchor
	// they were proven under and the Directory's receipt for the block (nil
	// on the Directory itself, whose root is the terminal). A bundle answering
	// a healing request is proven under the same anchor.
	Dispatched       bool
	DispatchedAt     uint64 // the producer's newest block when it dispatched
	AnchorBlock      uint64
	DirectoryReceipt *protocol.PartitionAnchorReceipt
}

// InFlightBlocks is how many of the producer's blocks a dispatched block is
// considered in flight: its entries are on their way and are not served to a
// healing request, so an answer never duplicates a delivery that is about to
// arrive. Two activations of the healing cadence.
const InFlightBlocks = 8

// Proof is the receipt from the entry at index to the anchor the block was
// dispatched under: through the synthetic chain, the root chain and, off the
// Directory, the Directory's receipt. Nil until the block is dispatched.
func (b *Block) Proof(index int64) (*merkle.Receipt, error) {
	if !b.Dispatched || b.Segment == nil || b.RootReceipt == nil {
		return nil, nil
	}
	synth, err := b.Segment.Receipt(index, b.Segment.Last())
	if err != nil {
		return nil, err
	}
	if b.DirectoryReceipt == nil {
		return synth.Combine(b.RootReceipt)
	}
	return synth.Combine(b.RootReceipt, b.DirectoryReceipt.RootChainReceipt)
}

// Continuation is what a receipt list over the block's entries continues
// with to reach the anchor the block was dispatched under. Nil until then.
func (b *Block) Continuation() (*merkle.Receipt, error) {
	if !b.Dispatched || b.RootReceipt == nil {
		return nil, nil
	}
	if b.DirectoryReceipt == nil {
		return b.RootReceipt, nil
	}
	return b.RootReceipt.Combine(b.DirectoryReceipt.RootChainReceipt)
}

// A ReceivedAnchor is a Directory anchor this partition executed, kept for
// the next block's open to dispatch the synthetics its receipts cover.
type ReceivedAnchor struct {
	Block  uint64 // the block that executed it
	Anchor *protocol.DirectoryAnchor
}

// Cache is one partition's producer cache. Block execution writes it through
// a Txn that commits with the block; the sequencer reads it concurrently.
type Cache struct {
	mu       sync.RWMutex
	horizon  uint64
	entries  map[string]map[uint64]*Entry // stream -> number
	byHash   map[[32]byte]*Entry
	blocks   map[uint64]*Block
	anchors  map[uint64]*anchorEntry // produced anchors by sequence number
	received []*ReceivedAnchor
	newest   uint64

	// the newest anchor produced, by sequence number, and the block it
	// anchored: what the next block's open asks before producing another
	lastAnchorNumber uint64
	lastAnchorBlock  uint64
	lastAnchorOK     bool
}

type anchorEntry struct {
	block uint64 // the block that produced it
	txn   *protocol.Transaction
}

// Counters is a snapshot of the cache's hit and miss counts by what was
// asked, for tests and reports; the same numbers are exported as metrics.
type Counters struct {
	Hits, Misses map[string]uint64
}

var (
	countMu   sync.Mutex
	countHits = map[string]uint64{}
	countMiss = map[string]uint64{}
)

// Stats reports the process-wide hit and miss counts by kind.
func Stats() Counters {
	countMu.Lock()
	defer countMu.Unlock()
	c := Counters{Hits: map[string]uint64{}, Misses: map[string]uint64{}}
	for k, v := range countHits {
		c.Hits[k] = v
	}
	for k, v := range countMiss {
		c.Misses[k] = v
	}
	return c
}

// New returns an empty cache keeping horizon blocks (DefaultHorizon when 0).
func New(horizon uint64) *Cache {
	if horizon == 0 {
		horizon = DefaultHorizon
	}
	return &Cache{
		horizon: horizon,
		entries: map[string]map[uint64]*Entry{},
		byHash:  map[[32]byte]*Entry{},
		blocks:  map[uint64]*Block{},
		anchors: map[uint64]*anchorEntry{},
	}
}

func streamKey(u *url.URL) string { return strings.ToLower(u.String()) }

// A Txn collects one block's additions. Nothing is visible until Commit, and
// a discarded block leaves nothing behind, so the cache never holds an entry
// the chain does not.
type Txn struct {
	c             *Cache
	block         uint64
	entries       []*Entry
	blk           *Block
	anchors       []*anchorEntry
	anchorNumbers []uint64
	anchorBlocks  []uint64 // the block each anchor anchors
	received      []*ReceivedAnchor
}

// Begin starts the additions of block index.
func (c *Cache) Begin(index uint64) *Txn { return &Txn{c: c, block: index} }

// Add records a produced entry.
func (t *Txn) Add(e *Entry) {
	if t == nil {
		return
	}
	t.entries = append(t.entries, e)
}

// SetBlock records what the block's proofs are built from. Called once per
// block, at close, after the root chain is final.
func (t *Txn) SetBlock(b *Block) {
	if t == nil {
		return
	}
	t.blk = b
}

// AddAnchor records an anchor this partition produced, by sequence number,
// and the block it anchors.
func (t *Txn) AddAnchor(number, anchoredBlock uint64, txn *protocol.Transaction) {
	if t == nil {
		return
	}
	t.anchors = append(t.anchors, &anchorEntry{t.block, txn})
	t.anchorNumbers = append(t.anchorNumbers, number)
	t.anchorBlocks = append(t.anchorBlocks, anchoredBlock)
}

// AddReceived records a Directory anchor the block executed, for dispatch at
// the next block's open.
func (t *Txn) AddReceived(a *protocol.DirectoryAnchor) {
	if t == nil {
		return
	}
	t.received = append(t.received, &ReceivedAnchor{t.block, a})
}

// Commit makes the block's additions visible and trims what the horizon has
// passed.
func (t *Txn) Commit() {
	if t == nil {
		return
	}
	c := t.c
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range t.entries {
		k := streamKey(e.Stream)
		m := c.entries[k]
		if m == nil {
			m = map[uint64]*Entry{}
			c.entries[k] = m
		}
		m[e.Number] = e
		c.byHash[e.Hash] = e
	}
	if t.blk != nil {
		c.blocks[t.blk.Index] = t.blk
	}
	for i, a := range t.anchors {
		c.anchors[t.anchorNumbers[i]] = a
	}
	c.received = append(c.received, t.received...)
	for i := range t.anchors {
		if n := t.anchorNumbers[i]; !c.lastAnchorOK || n >= c.lastAnchorNumber {
			c.lastAnchorNumber, c.lastAnchorBlock, c.lastAnchorOK = n, t.anchorBlocks[i], true
		}
	}
	if t.block > c.newest {
		c.newest = t.block
	}
	c.trimLocked()
	mEntries.Set(float64(len(c.byHash)))
	mBlocks.Set(float64(len(c.blocks)))
}

// Discard drops the block's additions.
func (t *Txn) Discard() {
	if t == nil {
		return
	}
	*t = Txn{}
}

func (c *Cache) trimLocked() {
	if c.newest <= c.horizon {
		return
	}
	cutoff := c.newest - c.horizon
	for idx, b := range c.blocks {
		if idx >= cutoff {
			continue
		}
		for _, e := range b.Entries {
			if m := c.entries[streamKey(e.Stream)]; m != nil {
				delete(m, e.Number)
			}
			delete(c.byHash, e.Hash)
		}
		delete(c.blocks, idx)
	}
	for n, a := range c.anchors {
		if a.block < cutoff {
			delete(c.anchors, n)
		}
	}
}

// Seed inserts blocks rebuilt from the node's own chains at start, before
// the cache has seen a block committed: genesis produces through another
// executor, and a restarted node has produced blocks whose anchors have not
// returned. It is the cache's sync step, run once, and it does not touch what
// a running block has added.
func (c *Cache) Seed(blocks []*Block) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, b := range blocks {
		if _, held := c.blocks[b.Index]; held {
			continue
		}
		c.blocks[b.Index] = b
		for _, e := range b.Entries {
			k := streamKey(e.Stream)
			m := c.entries[k]
			if m == nil {
				m = map[uint64]*Entry{}
				c.entries[k] = m
			}
			m[e.Number] = e
			c.byHash[e.Hash] = e
		}
		if b.Index > c.newest {
			c.newest = b.Index
		}
	}
	mEntries.Set(float64(len(c.byHash)))
	mBlocks.Set(float64(len(c.blocks)))
}

// Block answers what block index's proofs are built from and its entries, as
// a copy whose slices are shared and never modified. A miss is counted: it
// is a defect, an undersized cache or a request for a block older than the
// horizon.
func (c *Cache) Block(index uint64) (*Block, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	b, ok := c.blocks[index]
	count("block", ok)
	if !ok {
		return nil, false
	}
	cp := *b
	return &cp, true
}

// MarkDispatched records the Directory anchor block index's synthetics were
// proven under when they left, and the Directory's receipt for the block
// (nil on the Directory). Healing proves its bundles under the same anchor.
func (c *Cache) MarkDispatched(index, anchorBlock uint64, receipt *protocol.PartitionAnchorReceipt) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.blocks[index]
	if !ok {
		return
	}
	b.Dispatched, b.DispatchedAt, b.AnchorBlock, b.DirectoryReceipt = true, c.newest, anchorBlock, receipt
}

// Newest is the newest block committed to the cache.
func (c *Cache) Newest() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.newest
}

// Servable answers whether a block's entries may be served to a healing
// request: dispatched, and not for the last InFlightBlocks blocks.
func (c *Cache) Servable(b *Block) bool {
	return b.Dispatched && c.Newest() >= b.DispatchedAt+InFlightBlocks
}

// Entry answers one produced entry by stream and number.
func (c *Cache) Entry(stream *url.URL, number uint64) (*Entry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[streamKey(stream)][number]
	count("entry", ok)
	return e, ok
}

// ByHash answers one produced entry by its hash, the healing request's
// vocabulary.
func (c *Cache) ByHash(hash [32]byte) (*Entry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.byHash[hash]
	count("hash", ok)
	return e, ok
}

// Anchor answers a produced anchor by sequence number.
func (c *Cache) Anchor(number uint64) (*protocol.Transaction, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	a, ok := c.anchors[number]
	count("anchor", ok)
	if !ok {
		return nil, false
	}
	return a.txn, true
}

// LastAnchoredBlock is the block the newest produced anchor anchors, and
// whether any anchor has been produced since the cache began.
func (c *Cache) LastAnchoredBlock() (uint64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastAnchorBlock, c.lastAnchorOK
}

// TakeReceived drains the Directory anchors executed since the last call, in
// execution order. Dispatch at a block's open takes them.
func (c *Cache) TakeReceived() []*ReceivedAnchor {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := c.received
	c.received = nil
	return r
}

// Len reports how many entries and blocks are held.
func (c *Cache) Len() (entries, blocks int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.byHash), len(c.blocks)
}

func count(kind string, hit bool) {
	countMu.Lock()
	if hit {
		countHits[kind]++
	} else {
		countMiss[kind]++
	}
	countMu.Unlock()
	if hit {
		mHits.WithLabelValues(kind).Inc()
	} else {
		mMisses.WithLabelValues(kind).Inc()
	}
}

var (
	mHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "accumulate", Subsystem: "synthcache", Name: "hits_total",
		Help: "Reads the producer's synthetic/anchor cache answered, by what was asked: block, entry, hash, anchor",
	}, []string{"kind"})
	mMisses = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "accumulate", Subsystem: "synthcache", Name: "misses_total",
		Help: "Reads the producer's synthetic/anchor cache could not answer. A miss is a defect: the cache is undersized or the request is stale",
	}, []string{"kind"})
	mEntries = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "accumulate", Subsystem: "synthcache", Name: "entries",
		Help: "Produced entries held",
	})
	mBlocks = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "accumulate", Subsystem: "synthcache", Name: "blocks",
		Help: "Blocks held, each with what its proofs are built from",
	})
)
