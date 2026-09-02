// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"sort"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// streamPosition is where one stream stands — how far it has been delivered,
// and which numbers above that the executor is holding.
//
// The two halves come from different places, and that is the point (#4189).
// `delivered` is read from the stream's ledger, because what a block delivered
// is its output and belongs in state the block wrote. What is HELD is read from
// [execute.Staging], because that is the executor's own account of what it has
// taken in from consensus, and nothing the block writes may feed back into it.
//
// It used to be one thing: the block's working copy of a ledger entry whose
// Pending array was the held set. Being in an account forced that array to be
// bounded, and past the bound the executor stored a message while refusing to
// record that it had it — so the node held a message and reported not holding
// it, and healing fetched back across the partition what was already in the
// local database.
//
// Read once per stream per block through Block.positionOf. The executor used to
// read the ledger inside every message's own child batch, and a child does not
// share its parent's value, so each read deep-copied the whole ledger — every
// stream, every staged entry. Per message that was O(total backlog), and
// draining n cost O(n^2). See TestSequenceLedgerCostIsPerRead.
type streamPosition struct {
	stream    stream
	delivered uint64

	// batch is where held messages actually live — the block's own batch, so a
	// receipt recorded here commits with the block and is discarded with it.
	// The position reads through it rather than holding a copy: a copy is a
	// snapshot, and the whole defect this replaces was a snapshot of what the
	// node holds disagreeing with what the node holds.
	batch *database.Batch

	// highest is the largest number this stream has delivered in THIS block, or
	// zero if none. It is what the flush writes.
	highest uint64

	// err is the first failure from a read that could not report one — idOf is
	// called from buildRun, which is pure and total by design. The caller
	// checks it once, after the run is built.
	err error
}

// next is the number this stream is waiting for.
func (p *streamPosition) next() uint64 { return p.delivered + 1 }

// idOf returns the staged message ID for a sequence number, if we hold one.
//
// Held is "received, and above the watermark". The record survives delivery —
// nothing is deleted, because Delivered is the cutoff — so the watermark check
// is what makes this mean HELD rather than merely SEEN.
func (p *streamPosition) idOf(n uint64) (*url.TxID, bool) {
	if n <= p.delivered {
		return nil, false
	}
	id, ok, err := execute.IDOf(p.batch, p.stream.id(), n)
	if err != nil && p.err == nil {
		p.err = err
	}
	return id, ok
}

// has reports whether we hold a staged message for this number.
func (p *streamPosition) has(n uint64) bool {
	_, ok := p.idOf(n)
	return ok
}

// received is the largest number this stream has ever seen. It says the stream
// is behind; it does not say what is missing, which is execute.Missing.
func (p *streamPosition) received() uint64 {
	h, err := execute.Sighted(p.batch, p.stream.id())
	if err != nil && p.err == nil {
		p.err = err
	}
	if h > p.delivered {
		return h
	}
	return p.delivered
}

// positionCache holds the block's stream positions. Its mutex is why it lives
// behind a pointer on Block — see the field's comment.
type positionCache struct {
	mu sync.Mutex
	m  map[string]*streamPosition
}

func (s stream) key() string { return s.ledger.String() + "|" + s.source.String() }

// id is this stream's name in the executor's staging store.
func (s stream) id() execute.StreamID {
	return execute.StreamID{Ledger: s.ledger, Source: s.source}
}

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

	// Only Delivered. The rest of the entry is the source's Produced count and
	// the residue of the old design, and neither says anything about what this
	// node is holding right now.
	p := &streamPosition{
		stream:    s,
		delivered: ledger.Partition(s.source).Delivered,
		batch:     b.Batch,
	}
	if b.positions.m == nil {
		b.positions.m = map[string]*streamPosition{}
	}
	b.positions.m[key] = p
	return p, nil
}

// advanceStream records one advance of a stream: a delivery of the next
// number, or a receipt held for a later block.
//
// A delivery moves a watermark that does not self-correct, and every node on
// the other path is on a different chain, so a delivery out of order is refused
// rather than applied. A RECEIPT is refused only when it is behind the
// watermark, which is a caller error and not a state to be represented.
//
// There is no upper bound. There used to be one — MaxPendingSequenced, 4,096 —
// and it existed because the held set was an array in a record hashed into the
// BPT every block. Past it the executor logged at Debug and returned nil,
// storing the message and refusing to record that it had it. Bounding receipts
// bounded nothing real: the bodies were stored regardless, so the cap discarded
// only the INDEX of what the node held, and healing spent the network fetching
// it back. Staging is not hashed and not written, so there is nothing left to
// bound.
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
	}

	if !delivered {
		// Into the block's batch, so it commits with the block. A receipt
		// recorded by a block that is then discarded is discarded with it,
		// which is what makes "the node holds it" and "the node says it holds
		// it" the same statement.
		err = execute.Hold(b.Batch, s.id(), n, id)
		return errors.UnknownError.Wrap(err)
	}

	p.delivered = n
	p.highest = n
	return nil
}

// flushStreams writes each advanced stream's Delivered back to its ledger: one
// read, one assignment, one put, once per stream per block. That is where the
// O(n^2) drain went.
//
// ONLY Delivered. Received and Pending are not written and not maintained —
// they describe what the executor has taken in, which is staging's to say, and
// writing them here is exactly the feedback from block output into staging that
// #4189 removes. The record is re-read rather than overwritten from a copy
// because other things write it during the block (production bumps Produced),
// and those must survive.
//
// Streams are flushed in a fixed order; the state does not depend on it, but
// every node deriving the same thing the same way is cheap insurance.
func (b *Block) flushStreams() error {
	b.positions.mu.Lock()
	defer b.positions.mu.Unlock()

	keys := make([]string, 0, len(b.positions.m))
	for k, p := range b.positions.m {
		if p.highest > 0 {
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
		if p.highest > part.Delivered {
			part.Delivered = p.highest
		}
		err = b.Batch.Account(p.stream.ledger).Main().Put(ledger)
		if err != nil {
			return errors.UnknownError.WithFormat("store %v: %w", p.stream.ledger, err)
		}
	}
	return nil
}
