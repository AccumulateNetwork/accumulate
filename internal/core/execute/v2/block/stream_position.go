// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"sort"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// streamPosition is where one stream stands — how far it has been delivered,
// how far it has been received, and which numbers in between are staged:
// received, held, waiting for the gap ahead of them to close.
//
// It is the block's WORKING COPY of the stream's ledger entry (#4169 step 7).
// Read once per stream per block through Block.positionOf, advanced in place
// as the block executes, and written back once when the block closes. The
// executor used to read the ledger inside every message's own child batch,
// and a child does not share its parent's value, so each read deep-copied the
// whole ledger — every stream, every staged entry. Per message that was
// O(total backlog), and draining n cost O(n^2). See
// TestSequenceLedgerCostIsPerRead. Now the copy is paid once per stream per
// block and every message pays only its own Add.
type streamPosition struct {
	stream    stream
	delivered uint64
	received  uint64

	// staged is indexed by offset from delivered: staged[i] is number
	// delivered+1+i. A nil entry is a number we know was received — because
	// something past it arrived — but whose message we do not hold.
	staged []*url.TxID

	// work is the private copy this position is derived from. Private,
	// because the batch memoizes the record it was read from, and advancing
	// that in place would write the block's state behind the batch's back.
	work *protocol.PartitionSyntheticLedger

	// ops is every Add applied to work, in order, so the flush can replay
	// them onto the real record. The record is NOT overwritten with work:
	// other things write it during the block (production bumps Produced), and
	// a replay preserves those where a put of the copy would clobber them.
	ops []streamOp
}

// streamOp is one advance of a stream: a delivery, or a receipt recorded
// pending.
type streamOp struct {
	delivered bool
	number    uint64
	id        *url.TxID
}

// next is the number this stream is waiting for.
func (p *streamPosition) next() uint64 { return p.delivered + 1 }

// idOf returns the staged message ID for a sequence number, if we hold one.
//
// This is PartitionSyntheticLedger.Get restated, and it is restated rather than
// called so the offset arithmetic — index = n - delivered - 1 — lives in one
// place. Getting it wrong reads a neighbouring stream position and is the
// failure the positional Pending array invites.
func (p *streamPosition) idOf(n uint64) (*url.TxID, bool) {
	if n <= p.delivered || n > p.received {
		return nil, false
	}
	i := n - p.delivered - 1
	if i >= uint64(len(p.staged)) {
		return nil, false // received is ahead of the window we actually hold
	}
	id := p.staged[i]
	return id, id != nil
}

// has reports whether we hold a staged message for this number.
func (p *streamPosition) has(n uint64) bool {
	_, ok := p.idOf(n)
	return ok
}

// sync re-derives the position from the working copy after an advance.
func (p *streamPosition) sync() {
	p.delivered, p.received, p.staged = p.work.Delivered, p.work.Received, p.work.Pending
}

// positionCache holds the block's stream positions. Its mutex is why it lives
// behind a pointer on Block — see the field's comment.
type positionCache struct {
	mu sync.Mutex
	m  map[string]*streamPosition
}

func (s stream) key() string { return s.ledger.String() + "|" + s.source.String() }

// positionOf returns where a stream stands, loading it at most once per block.
//
// Guarded, because a cache MISS writes the map, and an advance writes the
// entry. Every caller today is in the block's serial phase, so nothing races
// right now — but "safe because no caller is concurrent yet" is a property of
// the callers, not of this code, and #4169 step 9 routes components to shards.
// A shard needing a position would have corrupted the map with no symptom
// until a block hash diverged. TestStreamPosition_ConcurrentReadsAreSafe
// fails under -race without this.
//
// The lock makes the CACHE safe, not the load behind it: a miss reads
// b.Batch, and the parent batch is only safe to touch from the serial phase
// (exec_parallel.go, hazard iv). So a shard may read a position that is
// already cached; it must not be the first to ask for one. Prefetching every
// stream's position while classifying would close that too, and is the right
// move if step 9 ever needs it.
func (b *Block) positionOf(s stream) (*streamPosition, error) {
	b.positions.mu.Lock()
	defer b.positions.mu.Unlock()
	return b.positionOfLocked(s)
}

func (b *Block) positionOfLocked(s stream) (*streamPosition, error) {
	if !s.ok() {
		return nil, errors.InternalError.With("not a stream")
	}
	key := s.key()
	if p, ok := b.positions.m[key]; ok {
		return p, nil
	}

	var ledger protocol.SequenceLedger
	err := b.Batch.Account(s.ledger).Main().GetAs(&ledger)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load %v: %w", s.ledger, err)
	}

	p := &streamPosition{stream: s, work: ledger.Partition(s.source).Copy()}
	p.sync()
	if b.positions.m == nil {
		b.positions.m = map[string]*streamPosition{}
	}
	b.positions.m[key] = p
	return p, nil
}

// advanceStream records one advance of a stream: a delivery of the next
// number, or a receipt recorded pending. It is the executor's ledger write,
// moved out of the message path (#4169 step 7) — the position moves now, so
// the next ask in this block sees it, and the record is written at close.
//
// This is the one write in the restructure that can corrupt state: a wrong
// advance moves a watermark that does not self-correct, and every node on the
// other path is on a different chain. So both ways of being wrong are refused
// rather than applied. Add would shift the pending window under a re-delivery
// and panic on a receipt behind the watermark; neither may reach it.
func (b *Block) advanceStream(s stream, delivered bool, n uint64, id *url.TxID) error {
	b.positions.mu.Lock()
	defer b.positions.mu.Unlock()

	p, err := b.positionOfLocked(s)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	switch {
	case delivered && n != p.next():
		return errors.FatalError.WithFormat("%v: delivered %d out of order: %d is next", s.source, n, p.next())

	case !delivered && n <= p.delivered:
		return errors.FatalError.WithFormat("%v: processed out of order: delivered %d, processed %d", s.source, p.delivered, n)

	case !delivered && n > p.delivered+MaxPendingSequenced:
		// Bounding must survive. Refusing to RECORD a receipt far beyond the
		// delivery point is deterministic — same rule, same state on every
		// validator — and converts unbounded receipt-state growth into a
		// produced>received tail at the source, which the reconcile machinery
		// heals once delivery catches up.
		b.Executor.logger.Debug("Refusing to record far-future sequenced message",
			"seq", n, "delivered", p.delivered, "window", MaxPendingSequenced, "source", s.source)
		return nil
	}

	p.work.Add(delivered, n, id)
	p.ops = append(p.ops, streamOp{delivered: delivered, number: n, id: id})
	p.sync()
	return nil
}

// flushStreams writes each advanced stream back to its ledger: one read, the
// block's Adds replayed in order, one put. Once per stream per block, which is
// where the O(n^2) drain went.
//
// A replay rather than a put of the working copy, so writes other code made to
// the same record during the block survive — see streamPosition.ops. Streams
// are flushed in a fixed order; the state does not depend on it, but every
// node deriving the same thing the same way is cheap insurance.
func (b *Block) flushStreams() error {
	b.positions.mu.Lock()
	defer b.positions.mu.Unlock()

	keys := make([]string, 0, len(b.positions.m))
	for k, p := range b.positions.m {
		if len(p.ops) > 0 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	for _, k := range keys {
		p := b.positions.m[k]
		var ledger protocol.SequenceLedger
		err := b.Batch.Account(p.stream.ledger).Main().GetAs(&ledger)
		if err != nil {
			return errors.UnknownError.WithFormat("load %v: %w", p.stream.ledger, err)
		}
		part := ledger.Partition(p.stream.source)
		for _, op := range p.ops {
			part.Add(op.delivered, op.number, op.id)
		}
		err = b.Batch.Account(p.stream.ledger).Main().Put(ledger)
		if err != nil {
			return errors.UnknownError.WithFormat("store %v: %w", p.stream.ledger, err)
		}
		p.ops = nil
	}
	return nil
}
