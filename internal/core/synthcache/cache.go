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

// DefaultHorizon is how many blocks a produced entry is kept at most: ten
// minutes at one block a second. It is the backstop, not the mechanism: the
// destination's Delivered, carried on every synthetic and every anchor it
// dispatches, releases what it has executed as soon as the word arrives
// (healing spec, "The cache"), so the horizon only binds a stream whose
// destination says nothing — one that produces neither synthetics nor
// anchors back — and bounds what such a stream can cost.
const DefaultHorizon = 600

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
	Index   uint64
	Streams map[string]*Stream // by destination: this block's span of each synthetic chain
	Entries []*Entry           // in production order

	// Set when the block's synthetics were dispatched: the Directory anchor
	// they were proven under and the Directory's receipt for the block (nil
	// on the Directory itself, whose root is the terminal). A bundle answering
	// a healing request is proven under the same anchor.
	Dispatched       bool
	DispatchedAt     uint64 // the producer's newest block when it dispatched
	AnchorBlock      uint64
	DirectoryReceipt *protocol.PartitionAnchorReceipt
}

// A Stream is this block's span of one destination's synthetic chain
// (executor spec, "One chain per pair, one stage per chain"): the chain's
// state before the block's first entry for that destination and the entries
// since, the receipt from the chain's anchor in the root chain to the root at
// close, and where that anchor landed. A proof for the destination's entries
// is built from this and nothing else; its index is the sequence number.
type Stream struct {
	Destination *url.URL
	ChainName   string
	Segment     *merkle.Segment
	RootReceipt *merkle.Receipt
	RootPos     int64  // the chain's anchor's position in the root chain
	IndexIndex  uint64 // the chain's index chain entry for this block
}

// Stream is the block's span of the chain to dst, or nil.
func (b *Block) Stream(dst *url.URL) *Stream {
	if b == nil || b.Streams == nil {
		return nil
	}
	return b.Streams[streamKey(dst)]
}

// InFlightBlocks is how many of the producer's blocks a dispatched block is
// considered in flight: its entries are on their way and are not served to a
// healing request, so an answer never duplicates a delivery that is about to
// arrive. Two activations of the healing cadence.
const InFlightBlocks = 8

// Proof is the receipt from the entry at index to the anchor the block was
// dispatched under: through the synthetic chain, the root chain and, off the
// Directory, the Directory's receipt. Nil until the block is dispatched.
func (b *Block) Proof(dst *url.URL, index int64) (*merkle.Receipt, error) {
	st := b.Stream(dst)
	if !b.Dispatched || st == nil || st.Segment == nil || st.RootReceipt == nil {
		return nil, nil
	}
	synth, err := st.Segment.Receipt(index, st.Segment.Last())
	if err != nil {
		return nil, err
	}
	if b.DirectoryReceipt == nil {
		return synth.Combine(st.RootReceipt)
	}
	return synth.Combine(st.RootReceipt, b.DirectoryReceipt.RootChainReceipt)
}

// Continuation is what a receipt list over the block's entries continues
// with to reach the anchor the block was dispatched under. Nil until then.
func (b *Block) Continuation(dst *url.URL) (*merkle.Receipt, error) {
	st := b.Stream(dst)
	if !b.Dispatched || st == nil || st.RootReceipt == nil {
		return nil, nil
	}
	if b.DirectoryReceipt == nil {
		return st.RootReceipt, nil
	}
	return st.RootReceipt.Combine(b.DirectoryReceipt.RootChainReceipt)
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
	released map[string]uint64 // per stream, the number the destination has said it executed through

	// per destination of this partition's anchors, the anchor number it has
	// said it executed through; an anchor every destination has executed is
	// released
	anchorAcks map[string]uint64

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

// StreamKey is the key a destination's stream is held under in Block.Streams.
func StreamKey(dst *url.URL) string { return streamKey(dst) }

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
	released      map[string]release
	anchorAcks    map[string]anchorAck
}

type release struct {
	stream  *url.URL
	through uint64
}

// anchorAck is a destination's word on this partition's anchors: it has
// executed them through number `through`. fanout is how many destinations
// this partition anchors to — one for a BVN, every partition for the
// Directory — since an anchor goes to all of them under one number and is
// released only when the last of them has executed it.
type anchorAck struct {
	through uint64
	fanout  int
}

// ReleaseAnchors records that destination dst has executed this partition's
// anchors through `through` (healing spec, "The cache"). At commit, anchors
// every one of the fanout destinations has executed are dropped.
func (t *Txn) ReleaseAnchors(dst *url.URL, through uint64, fanout int) {
	if t == nil || dst == nil || through == 0 || fanout <= 0 {
		return
	}
	if t.anchorAcks == nil {
		t.anchorAcks = map[string]anchorAck{}
	}
	k := streamKey(dst)
	if a, ok := t.anchorAcks[k]; !ok || through > a.through {
		t.anchorAcks[k] = anchorAck{through, fanout}
	}
}

// Release records that the destination has executed this partition's stream
// to it through number `through`: it will never ask for anything at or below,
// so the entries and the block segments that held them go at commit (healing
// spec, "The cache").
func (t *Txn) Release(stream *url.URL, through uint64) {
	if t == nil || stream == nil {
		return
	}
	if t.released == nil {
		t.released = map[string]release{}
	}
	k := streamKey(stream)
	if r, ok := t.released[k]; !ok || through > r.through {
		t.released[k] = release{stream, through}
	}
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
	for k, r := range t.released {
		c.releaseLocked(k, r.through)
	}
	for k, a := range t.anchorAcks {
		c.releaseAnchorsLocked(k, a)
	}
	c.trimLocked()
	mEntries.Set(float64(len(c.byHash)))
	mBlocks.Set(float64(len(c.blocks)))
}

// releaseAnchorsLocked records a destination's word and drops every anchor
// all fanout destinations have executed. The walk is over the anchors held,
// never over the number line, so a claim costs what the cache holds.
func (c *Cache) releaseAnchorsLocked(k string, a anchorAck) {
	if c.anchorAcks == nil {
		c.anchorAcks = map[string]uint64{}
	}
	if a.through > c.anchorAcks[k] {
		c.anchorAcks[k] = a.through
	}
	if len(c.anchorAcks) < a.fanout {
		return // a destination has not spoken yet; nothing is safe to drop
	}
	through := a.through
	for _, n := range c.anchorAcks {
		if n < through {
			through = n
		}
	}
	var dropped int
	for n := range c.anchors {
		if n <= through {
			delete(c.anchors, n)
			dropped++
		}
	}
	mAnchorsReleased.Add(float64(dropped))
}

// Discard drops the block's additions.
func (t *Txn) Discard() {
	if t == nil {
		return
	}
	*t = Txn{}
}

// releaseLocked drops a stream's entries through number n and the block
// segments that held only released entries. Numbers are dense per stream, so
// the walk is over what is newly released, not over what is held.
func (c *Cache) releaseLocked(k string, n uint64) {
	if c.released == nil {
		c.released = map[string]uint64{}
	}
	from := c.released[k] + 1
	if n < from {
		return
	}
	m := c.entries[k]
	if m == nil {
		return // nothing held for the stream: a claim over nothing is not recorded
	}
	// The claim is the destination's word, not ours to trust with the walk:
	// it is clamped to what is held, and when it outruns the held set the
	// held set is walked instead, so a forged or post-restart value costs
	// what the cache holds, never what the number says (second-pass review
	// 2026-09-06). A claim past the highest held number releases everything
	// held; nothing above it exists to release.
	var highest uint64
	for num := range m {
		if num > highest {
			highest = num
		}
	}
	if n > highest {
		n = highest
	}
	c.released[k] = n
	touched := map[uint64]bool{}
	var dropped int
	drop := func(num uint64, e *Entry) {
		delete(m, num)
		delete(c.byHash, e.Hash)
		touched[e.Block] = true
		dropped++
	}
	if n < from {
		return
	}
	if n-from+1 > uint64(len(m)) {
		for num, e := range m {
			if num >= from && num <= n {
				drop(num, e)
			}
		}
	} else {
		for num := from; num <= n; num++ {
			if e, ok := m[num]; ok {
				drop(num, e)
			}
		}
	}
	for idx := range touched {
		b := c.blocks[idx]
		if b == nil {
			continue
		}
		kept := b.Entries[:0]
		for _, e := range b.Entries {
			if streamKey(e.Stream) != k || e.Number > n {
				kept = append(kept, e)
			}
		}
		clear(b.Entries[len(kept):])
		b.Entries = kept
		if st := b.Streams[k]; st != nil && st.Segment != nil && uint64(st.Segment.Last())+1 <= n {
			delete(b.Streams, k) // its last entry is released: nothing is proven from it again
		}
		if len(b.Entries) == 0 && len(b.Streams) == 0 {
			// Nothing left to prove: the block's header and its Directory
			// receipt have no reader (Proof and Continuation go through a
			// stream), and kept to the horizon they were the residue
			// (review 2026-09-06, finding 30)
			delete(c.blocks, idx)
		}
	}
	mReleased.Add(float64(dropped))
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
	b.Dispatched, b.DispatchedAt, b.AnchorBlock = true, c.newest, anchorBlock
	if len(b.Entries) > 0 || len(b.Streams) > 0 {
		// The receipt is what a proof continues through; a block that
		// produced nothing has nothing to prove, and its receipt kept to the
		// horizon was the residue (review 2026-09-06, finding 30). The header
		// itself stays: dispatch looks the block up, on the Directory twice.
		b.DirectoryReceipt = receipt
	}
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

// Entry answers one produced entry by stream and number. A miss is counted.
func (c *Cache) Entry(stream *url.URL, number uint64) (*Entry, bool) {
	e, ok := c.Peek(stream, number)
	count("entry", ok)
	return e, ok
}

// Peek answers one produced entry by stream and number without counting the
// outcome: for a reader that knows a number may not have been produced yet,
// and counts the miss itself only when it was (healing spec, "The answer":
// asked too soon is "not yet", not a miss).
func (c *Cache) Peek(stream *url.URL, number uint64) (*Entry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[streamKey(stream)][number]
	return e, ok
}

// Count records a hit or a miss of the given kind, for a reader that looked
// with Peek and has decided what the outcome was.
func Count(kind string, hit bool) { count(kind, hit) }

// ByHash answers one produced entry by its hash, the healing request's
// vocabulary.
func (c *Cache) ByHash(hash [32]byte) (*Entry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.byHash[hash]
	count("hash", ok)
	return e, ok
}

// Anchor answers a produced anchor by sequence number, and the block that
// recorded it. A miss is counted.
func (c *Cache) Anchor(number uint64) (*protocol.Transaction, uint64, bool) {
	txn, block, ok := c.PeekAnchor(number)
	count("anchor", ok)
	return txn, block, ok
}

// PeekAnchor is Anchor without counting the outcome; see Peek.
func (c *Cache) PeekAnchor(number uint64) (*protocol.Transaction, uint64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	a, ok := c.anchors[number]
	if !ok {
		return nil, 0, false
	}
	return a.txn, a.block, true
}

// LastAnchorNumber is the sequence number of the newest anchor produced since
// the cache began, and whether there is one.
func (c *Cache) LastAnchorNumber() (uint64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastAnchorNumber, c.lastAnchorOK
}

// LastAnchoredBlock is the block the newest produced anchor anchors, and
// whether any anchor has been produced since the cache began.
func (c *Cache) LastAnchoredBlock() (uint64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastAnchorBlock, c.lastAnchorOK
}

// ReceivedPending is how many Directory anchors wait to be taken.
func (c *Cache) ReceivedPending() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.received)
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

// AnchorLen reports how many produced anchors are held.
func (c *Cache) AnchorLen() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.anchors)
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
	mReleased = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "accumulate",
		Subsystem: "synthcache",
		Name:      "released_total",
		Help:      "Entries dropped because the destination said it had executed them (the Delivered carried on its dispatch)",
	})
	mAnchorsReleased = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "accumulate",
		Subsystem: "synthcache",
		Name:      "anchors_released_total",
		Help:      "Produced anchors dropped because every destination said it had executed them (the Delivered carried on its anchors)",
	})
	mEntries = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "accumulate", Subsystem: "synthcache", Name: "entries",
		Help: "Produced entries held",
	})
	mBlocks = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "accumulate", Subsystem: "synthcache", Name: "blocks",
		Help: "Blocks held, each with what its proofs are built from",
	})
)
