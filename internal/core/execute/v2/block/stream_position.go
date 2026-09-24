// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"sort"
	"strings"
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
	// proven answers whether the proven set covers a hash; nil means every
	// held entry is runnable (tests without an executor).
	// staging is the block's view of what is held and proven
	staging *execute.StagingTxn

	// batch is where held messages actually live — the block's own batch, so a
	// receipt recorded here commits with the block and is discarded with it.
	// The position reads through it rather than holding a copy: a copy is a
	// snapshot, and the whole defect this replaces was a snapshot of what the
	// node holds disagreeing with what the node holds.
	batch *database.Batch

	// highest is the largest number this stream has delivered in THIS block, or
	// zero if none. It is what the flush writes.
	highest uint64

	block *Block
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
	h, ok := p.staging.IDOf(p.stream.id(), n)
	if !ok {
		return nil, false
	}
	return h.ID, true
}

// heldAt is what staging holds at n, if anything.
func (p *streamPosition) heldAt(n uint64) *execute.Held {
	if n <= p.delivered {
		return nil
	}
	h, _ := p.staging.IDOf(p.stream.id(), n)
	return h
}

// has reports whether we hold a staged message for this number.
func (p *streamPosition) has(n uint64) bool {
	_, ok := p.idOf(n)
	return ok
}

// runnable reports whether a held number may be taken into a run. An entry
// held by the sequenced layer passed its proof when it was held; an entry
// COLLECTED without one is runnable only once the hash validated at its
// number is its own (executor spec, "Collection").
func (p *streamPosition) runnable(n uint64) bool {
	h := p.heldAt(n)
	if h == nil || !h.Collected {
		return true
	}
	if p.staging.IsValidated(p.stream.id(), n, h.Hash) {
		return true
	}
	// An anchor is also validated by a validator signature quorum, which
	// builds as its copies arrive (executor spec, "One chain per pair, one
	// stage per chain").
	if p.stream.kind == streamAnchor && p.block != nil {
		if seq, ok := h.Message.(*messaging.SequencedMessage); ok {
			if txn, ok := seq.Message.(*messaging.TransactionMessage); ok {
				ok, err := p.block.anchorIsAdmissible(p.batch, nil, txn.Transaction, p.stream.source)
				return err == nil && ok
			}
		}
	}

	// An entry collected WITH its own collection proof is runnable once that
	// proof's anchor has executed here: the same question its arrival asked,
	// asked again against the chain as it now stands (executor spec,
	// "Collection"). Its own proof is re-checked in full when it runs — that
	// it covers this message, and who signed it — so this decides only WHEN
	// it is offered, never whether it is valid.
	//
	// Without this, an entry held because its anchor had not arrived waits
	// for a PACKAGE proof over its number, which may never come: the proof it
	// arrived with is not staged for its anchor, and nothing re-offers it. On
	// a node that is joining, which decides what to hold against a state it
	// is still pulling, that is a stream that stops where its peers' moves —
	// which is divergence (#4294).
	if p.block != nil {
		if proof := heldCollectionProof(h); proof != nil {
			_, ok, err := p.block.Executor.provingAnchorIndex(p.batch, proof)
			return err == nil && ok
		}
	}
	return false
}

// heldCollectionProof is the proof a held entry arrived with, if it arrived
// with one of its own.
func heldCollectionProof(h *execute.Held) *protocol.AnnotatedReceipt {
	switch m := h.Message.(type) {
	case *messaging.SyntheticMessage:
		return m.Proof
	case *messaging.BadSyntheticMessage:
		return m.Proof
	}
	return nil
}

// received is the largest number this node's staging has ever seen on the
// stream. It says the stream is behind; it does not say what is missing, which
// is execute.Missing. It is staging's memory, so it is this node's: what the
// ledger records is [Block.hold]'s count, never this.
func (p *streamPosition) received() uint64 {
	if h := p.staging.Sighted(p.stream.id()); h > p.delivered {
		return h
	}
	return p.delivered
}

// positionCache holds the block's stream positions. Its mutex is why it lives
// behind a pointer on Block — see the field's comment.
type positionCache struct {
	mu sync.Mutex
	m  map[string]*streamPosition

	// received is, per stream, the highest number this block's own
	// execution handed to staging -- what the flush raises the ledger's
	// Received to (#4412). Kept apart from m because a hold can come from a
	// shard, which must not be the first to load a position.
	received map[string]receivedMark
}

// receivedMark is the highest number a block held on one stream.
type receivedMark struct {
	stream stream
	n      uint64
}

// key names the stream case-insensitively, as URLs are: one stream, one
// position, however its source was spelled.
func (s stream) key() string {
	return strings.ToLower(s.ledger.String()) + "|" + strings.ToLower(s.source.String())
}

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

	// Only Delivered. The rest of the entry is the source's Produced count and
	// the residue of the old design, and neither says anything about what this
	// node is holding right now. A stream whose ledger does not exist yet has
	// delivered nothing: zero is the answer, not an error.
	var delivered uint64
	var ledger protocol.SequenceLedger
	switch err := b.Batch.Account(s.ledger).Main().GetAs(&ledger); {
	case errors.Is(err, errors.NotFound):
	case err != nil:
		return nil, errors.UnknownError.WithFormat("load %v: %w", s.ledger, err)
	default:
		// A read, so FindPartition: Partition would insert an entry into
		// the batch's memoized record for a stream positioned by an arrival
		// that then writes nothing (#4412 review F8). An absent entry has
		// delivered nothing.
		if part, ok := ledger.FindPartition(s.source); ok {
			delivered = part.Delivered
		}
	}
	p := &streamPosition{
		stream:    s,
		delivered: delivered,
		batch:     b.Batch,
		staging:   b.staging,
		block:     b,
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
func (b *Block) advanceStream(s stream, delivered bool, n uint64, id *url.TxID, msg messaging.Message) error {
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
		// Held in memory, with the message itself — what runs when the
		// number is next — through the block's staging transaction, so a
		// discarded block leaves nothing behind (executor spec, "Sync")
		b.holdLocked(s, p.delivered, n, &execute.Held{ID: id, Message: msg})
		return nil
	}

	p.delivered = n
	p.highest = n
	return nil
}

// hold takes an entry of a stream into staging at its number, and counts the
// number towards the stream's Received (executor spec, "What the stream ledger
// is for"; #4412). Every hold the block's execution makes goes through here.
//
// `delivered` is the stream's Delivered as the caller read it from the ledger.
// The count is made HERE, from what the block's consensus messages carried,
// and not read back from staging: staging's own Hold also consults what this
// node's memory holds and has validated, and after a restart that memory is
// not its peers'. Received is hashed, so it may depend only on the state and
// on the block. A number at or below Delivered is not an arrival, and one past
// the stage's span is not held anywhere, so neither counts.
func (b *Block) hold(s stream, delivered, n uint64, h *execute.Held) {
	b.positions.mu.Lock()
	defer b.positions.mu.Unlock()
	b.holdLocked(s, delivered, n, h)
}

func (b *Block) holdLocked(s stream, delivered, n uint64, h *execute.Held) {
	b.staging.Hold(s.id(), n, h)
	if n <= delivered || n-delivered > execute.MaxStageSpan {
		return
	}
	k := s.key()
	if b.positions.received == nil {
		b.positions.received = map[string]receivedMark{}
	}
	if m, ok := b.positions.received[k]; !ok || n > m.n {
		b.positions.received[k] = receivedMark{s, n}
	}
}

// flushStreams writes each stream's Delivered and Received back to its ledger:
// one read, one assignment, one put, once per stream per block. That is where
// the O(n^2) drain went.
//
// Delivered is what the block ran. Received is the highest number that has
// entered staging by the end of the block: the ledger's own value, raised to
// the highest number this block held ([Block.hold]) and to Delivered (#4412).
// It never decreases and is never below Delivered. Both are derived from the
// state and from what consensus delivered in this block, so every validator
// writes the same values. Pending is not written: the held set is staging's.
// The record is re-read rather than overwritten from a copy because other
// things write it during the block (production bumps Produced), and those must
// survive.
//
// Streams are flushed in a fixed order; the state does not depend on it, but
// every node deriving the same thing the same way is cheap insurance.
func (b *Block) flushStreams() error {
	b.positions.mu.Lock()
	defer b.positions.mu.Unlock()

	// A stream held on is positioned here if it was not already. Nothing
	// reaches that today -- the block positions every stream it has an
	// arrival on before any message runs (exec_stage.go, stageRuns) -- but a
	// mark with no position would otherwise be dropped silently, and a
	// dropped mark is a wrong hashed value, not an error. Flush runs in the
	// block's serial phase, so loading a position here is safe.
	for _, m := range b.positions.received {
		if _, err := b.positionOfLocked(m.stream); err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	keys := make([]string, 0, len(b.positions.m))
	for k := range b.positions.m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		p := b.positions.m[k]
		mark := b.positions.received[k].n

		// Every stream the block touched releases at the ledger's Delivered,
		// not only the ones it delivered into. Staging is memory, so after a
		// restart its copy of Delivered is zero while the ledger's is not,
		// and a snapshot of staging that said zero would hand a joining node
		// entries its peers executed blocks ago (#4291). Releasing what the
		// ledger already says is delivered is a no-op for a stream that is
		// up to date and the correction for one that is not.
		if p.delivered > 0 {
			b.staging.Release(p.stream.id(), p.delivered)
		}
		err := b.raiseReceived(p.stream, p.highest, mark)
		if err != nil {
			return err
		}
	}
	return nil
}

// raiseReceived writes one stream's Delivered (when the block delivered, as
// highest) and Received (to at least the block's mark and Delivered). A
// stream the block neither delivered on nor raised is not written.
//
// A stream whose Received rises is counted in BlockState.ReceivedRaised, so
// such a block is never empty: an empty block's batch is discarded, and the
// raise would go with it. Today every hold comes from a message whose
// processing state already makes the block non-empty (MergeTransaction), so
// this is a guarantee that does not depend on that, not the mechanism that
// decides it.
func (b *Block) raiseReceived(s stream, highest, mark uint64) error {
	if highest == 0 && mark == 0 {
		return nil
	}
	var ledger protocol.SequenceLedger
	err := b.Batch.Account(s.ledger).Main().GetAs(&ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("load %v: %w", s.ledger, err)
	}
	part := ledger.Partition(s.source)
	changed := false
	if highest > part.Delivered {
		part.Delivered, changed = highest, true
	}
	received := max(part.Received, part.Delivered, mark)
	if received != part.Received {
		part.Received, changed = received, true
		b.State.ReceivedRaised++
	}
	if !changed {
		return nil
	}
	err = b.Batch.Account(s.ledger).Main().Put(ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("store %v: %w", s.ledger, err)
	}
	return nil
}
