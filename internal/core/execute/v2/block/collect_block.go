// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// CollectBlock takes a committed block into staging WITHOUT EXECUTING IT: the
// block's intake, and nothing else (executor spec, "Sync", step 1). It is what
// a joining node does with every block it receives from consensus while it is
// pulling the state down — and a restart is a join (#4205).
//
// The block's messages are sorted into streams exactly as a block sorts them,
// every proof the block carried goes through the same intake, and every
// arrival is held at its number in the form the block would hold it: a
// synthetic that passes its proof as the sequenced layer holds it, one that
// does not as collection holds it, an anchor copy as a copy below its quorum
// is held. NOTHING IS WRITTEN TO THE STORE — not an anchor signature, not a
// message body, not a ledger — because the state comes from the pull, not
// from here (#4293), and a collecting node has no block to commit.
//
// Two things make snapshot + collect exact. Holding dedupes by number, so a
// consensus copy of an entry the snapshot already holds is a no-op; and
// nothing is released here, so a block whose entries this node's store says
// are already delivered still holds them — SettleStaging drops exactly those
// once the pulled state says what `Delivered` is at Q.
func (x *Executor) CollectBlock(batch *database.Batch, params execute.BlockParams, envelopes []*messaging.Envelope) (*execute.CollectedBlock, error) {
	b := &Block{
		BlockParams: params,
		Executor:    x,
		Batch:       batch,
		staging:     x.staging().Begin(),
		positions:   new(positionCache),
	}

	c := b.classify(envelopes)

	// Group 0, as ProcessAll does it: every proof the block brought, before
	// any entry is judged against it. A refusal is the intake's own answer
	// and the entries it covered are skipped with it; anything else is the
	// store failing, and a node that staged less than its peers would
	// diverge.
	for _, p := range c.proofs {
		err := b.intakeProof(p.source, p.proof, p.siblings)
		if err != nil && !errors.Is(err, errors.BadRequest) {
			b.staging.Discard()
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	out := new(execute.CollectedBlock)
	keys := make([]string, 0, len(c.streams))
	for k := range c.streams {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return lessStream(c.streams[keys[i]], c.streams[keys[j]]) })
	for _, k := range keys {
		str := c.streams[k]
		pos, err := b.positionOf(str)
		if err != nil {
			b.staging.Discard()
			return nil, errors.UnknownError.Wrap(err)
		}
		// Release what the store already says is delivered, as closing a
		// block does for every stream it positioned. A collecting node never
		// closes a block, so without this its staging's own Delivered stays
		// at zero and the first entry it holds on a stream the peers have
		// delivered half a million of sizes the stage from zero (#4291).
		// Nothing at or below the store's Delivered can run again, and the
		// pulled state's Delivered only moves forward, so this is safe
		// before the settle as well as at it.
		if pos.delivered > 0 {
			b.staging.Release(str.id(), pos.delivered)
		}

		numbers := make([]uint64, 0, len(c.arrivals[k]))
		for n := range c.arrivals[k] {
			numbers = append(numbers, n)
		}
		sort.Slice(numbers, func(i, j int) bool { return numbers[i] < numbers[j] })
		counted := execute.CollectedStream{ID: str.id()}
		for _, n := range numbers {
			notHeld, err := b.collectArrival(str, pos.delivered, c.arrivals[k][n])
			if err != nil {
				b.staging.Discard()
				return nil, errors.UnknownError.Wrap(err)
			}
			if notHeld == "" {
				out.Held++
				counted.Held++
				continue
			}
			if counted.NotHeld == nil {
				counted.NotHeld = map[execute.NotHeldReason]int{}
			}
			counted.NotHeld[notHeld]++
		}
		out.Streams = append(out.Streams, counted)
	}

	b.staging.Commit()
	return out, nil
}

// CollectCommittedBlock is CollectBlock for a caller with no batch of its own
// — consensus, through the bridge. The batch is read-only in effect and is
// discarded either way: collecting writes nothing, and a batch left open
// would pin a version of the store for the life of the process (#4279).
func (x *Executor) CollectCommittedBlock(params execute.BlockParams, envelopes []*messaging.Envelope) (*execute.CollectedBlock, error) {
	batch := x.Database.Begin(false)
	defer batch.Discard()
	return x.CollectBlock(batch, params, envelopes)
}

// SettleStagingAt is SettleStaging for a caller with no batch of its own.
func (x *Executor) SettleStagingAt(q uint64) error {
	batch := x.Database.Begin(false)
	defer batch.Discard()
	return x.SettleStaging(batch, q)
}

// collectArrival holds one arrival the way the block that executes it would
// hold it, and answers why it was not held: empty when it was.
//
// The form matters as much as the fact. An entry that passes its proof is
// held by the sequenced layer as the sequenced message itself, not collected,
// and runs the moment its number is next; an entry that does not is held as
// collection holds it — the outer message, marked collected, with the hash a
// proof must validate, and it never runs until one does (executor spec,
// "Collection"). Holding the wrong one either strands an entry its peers ran
// or runs one its peers did not.
func (b *Block) collectArrival(str stream, delivered uint64, a *arrival) (execute.NotHeldReason, error) {
	// At or below what this node's store says it delivered, or past the
	// sanity horizon. Nothing below Delivered can ever run again, and the
	// pulled state decides the rest at Q.
	if a.seq.Number <= delivered {
		return execute.NotHeldDelivered, nil
	}
	if a.seq.Number > delivered+maxSequenceAhead {
		return execute.NotHeldHorizon, nil
	}
	if _, already := b.staging.IDOf(str.id(), a.seq.Number); already {
		return execute.NotHeldDuplicate, nil // first sighting wins, here as in a block
	}

	// The message's own executor decides whether it may be held at all, here
	// as in a block. Collecting must not hold what a block refuses: an entry
	// held at a number its peers hold nothing at is a number the peers' entry
	// can never take — first sighting wins — so the stream stops there for
	// good. It is also what keeps a stranger from sizing this node's stage
	// with a forged number (#4243).
	ctx := &MessageContext{bundle: &bundle{Block: b, batch: b.Batch, messages: a.bundle}, message: a.classifier}

	if str.kind == streamAnchor {
		return b.collectAnchor(str, ctx, a)
	}
	return b.collectSynthetic(str, ctx, a)
}

// collectSynthetic holds one synthetic arrival as SyntheticMessage.process
// would leave it: executed or held by the sequenced layer when its proof is
// anchored here, collected when it is not, refused when its own executor
// refuses it.
func (b *Block) collectSynthetic(str stream, ctx *MessageContext, a *arrival) (execute.NotHeldReason, error) {
	if _, ok := a.classifier.(*messaging.SyntheticMessage); !ok {
		if _, ok := a.classifier.(*messaging.BadSyntheticMessage); !ok {
			// A bare sequenced message: the replica-accepted case (#4140),
			// admissible on its own, held as the sequenced layer holds it.
			ok, err := b.admissibilityOf(str, a.classifier, a.seq)
			if err != nil || !ok {
				return execute.NotHeldRefused, nil
			}
			b.staging.Hold(str.id(), a.seq.Number, &execute.Held{ID: a.seq.ID(), Message: a.seq})
			return "", nil
		}
	}

	syn, attested, err := SyntheticMessage{}.check(b.Batch, ctx)
	if err != nil {
		// Refused by the rule the block refuses it by, and not held.
		return execute.NotHeldRefused, nil
	}

	if syn.Proof == nil {
		// Replica-accepted, or already covered by a validated proof: the
		// block executes it, and holds it by the sequenced layer if it is
		// not next.
		b.staging.Hold(str.id(), a.seq.Number, &execute.Held{ID: a.seq.ID(), Message: a.seq})
		return "", nil
	}

	proven, err := b.Executor.isAdmissible(b.Batch, syn.Proof)
	if err != nil {
		return "", errors.UnknownError.Wrap(err)
	}
	if proven {
		// The block absorbs an accepted collection proof into the stream's
		// replica (#4140) before it executes the message; a collecting node
		// must too, or the hashes it has validated are not the hashes its
		// peers have validated.
		if syn.Proof.ReceiptList != nil {
			err := b.staging.Prove(b.Executor.synthStream(a.seq.Source), syn.Proof.ReceiptList)
			if err != nil && !errors.Is(err, errors.Conflict) {
				return "", errors.UnknownError.Wrap(err)
			}
		}
		b.staging.Hold(str.id(), a.seq.Number, &execute.Held{ID: a.seq.ID(), Message: a.seq})
		return "", nil
	}

	// Not anchored here: collected, and only on a source validator's word
	// (#4243) — the same refusal the block makes.
	if !attested {
		return execute.NotHeldUnattested, nil
	}

	// Held whether or not its package's proof fitted the proof budget, as
	// collection holds it: the budget bounds memory, never what is held
	// (#4439).
	h := &execute.Held{ID: a.classifier.ID(), Message: a.classifier, Collected: true, Hash: a.seq.Hash()}
	if m, ok := a.seq.Message.(messaging.MessageForTransaction); ok {
		want := m.GetTxID().Hash()
		for _, sibling := range a.bundle {
			txn, ok := sibling.(messaging.MessageWithTransaction)
			if ok && txn.GetTransaction() != nil && txn.GetTransaction().ID().Hash() == want {
				h.Companion = sibling
				break
			}
		}
	}
	b.staging.Hold(str.id(), a.seq.Number, h)
	return "", nil
}

// collectAnchor holds one anchor copy as BlockAnchor.process would leave it:
// held by the sequenced layer once its signatures reach the threshold,
// collected below it, refused when its own executor refuses it.
func (b *Block) collectAnchor(str stream, ctx *MessageContext, a *arrival) (execute.NotHeldReason, error) {
	txn, ok := a.seq.Message.(*messaging.TransactionMessage)
	if !ok || txn.Transaction == nil {
		return execute.NotHeldRefused, nil
	}
	if _, ok := a.classifier.(*messaging.BlockAnchor); ok {
		if _, err := (BlockAnchor{}).check(ctx, b.Batch); err != nil {
			// Refused by the rule the block refuses it by, and not held.
			return execute.NotHeldRefused, nil
		}
	}

	// At its quorum the block executes it, and the sequenced layer holds it
	// if it is not next; below its quorum it is held as a copy below quorum
	// is held — the sequenced message, and the hash a proof over the source's
	// anchor chain validates, which is the transaction's stored form.
	ready, err := b.anchorIsAdmissible(b.Batch, nil, txn.Transaction, str.source)
	if err != nil {
		return execute.NotHeldRefused, nil
	}
	if ready {
		b.staging.Hold(str.id(), a.seq.Number, &execute.Held{ID: a.seq.ID(), Message: a.seq})
		return "", nil
	}

	stored := new(protocol.Transaction)
	stored.Body = txn.Transaction.Body
	b.staging.Hold(str.id(), a.seq.Number, &execute.Held{
		ID:        a.seq.ID(),
		Message:   a.seq,
		Collected: true,
		Hash:      *(*[32]byte)(stored.GetHash()),
	})
	return "", nil
}

// SettleStaging brings staging to the block the pulled state is (executor
// spec, "Sync", step 4). The node has collected every block through Q, its
// store is Q's state, and what is left is to make staging say what its peers'
// staging says at Q:
//
//   - every proof waiting on a Directory anchor block the state at Q has
//     executed is decided — validated or discarded — exactly as the anchor
//     group decides them in a block;
//   - every stream is released through the `Delivered` the PULLED LEDGER
//     names, not through anything staging remembers: what a block delivered
//     is block output, and the pulled state is the peers' word on it. Anchor
//     copies at or below the anchor ledger's `Delivered` go the same way.
//
// Q is checked against the store rather than trusted: a staging settled
// against a different block than the state it is paired with executes a
// different block than its peers, which is the whole failure this exists to
// prevent (#4290).
func (x *Executor) SettleStaging(batch *database.Batch, q uint64) error {
	var ledger *protocol.SystemLedger
	err := batch.Account(x.Describe.Ledger()).Main().GetAs(&ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("load %v: %w", x.Describe.Ledger(), err)
	}
	// The ledger's index is the last block that WROTE something: an empty
	// block writes nothing at all, not even its index, so the state at Q is
	// the state the last non-empty block at or below Q left. A ledger AHEAD
	// of Q is the error this exists to catch — staging settled against a
	// later state than the block it is paired with executes a different block
	// than its peers (#4290).
	if ledger.Index > q {
		return errors.Conflict.WithFormat("%s: cannot settle staging at block %d against state at block %d",
			x.Describe.PartitionId, q, ledger.Index)
	}

	b := &Block{
		BlockParams: execute.BlockParams{Index: q},
		Executor:    x,
		Batch:       batch,
		staging:     x.staging().Begin(),
		positions:   new(positionCache),
	}

	// The anchors executed by Q decide every proof waiting at or below the
	// Directory block they name, as they would have in the block that
	// executed them.
	through, err := b.directoryAnchorBlock()
	if err != nil {
		b.staging.Discard()
		return errors.UnknownError.Wrap(err)
	}
	err = b.decideProofs(nil, through)
	if err != nil {
		b.staging.Discard()
		return errors.UnknownError.Wrap(err)
	}

	// Release each stream through the pulled ledger's Delivered.
	streams := b.staging.Streams()
	released := 0
	for _, st := range streams {
		delivered, err := deliveredFrom(batch, st.ID)
		if err != nil {
			b.staging.Discard()
			return errors.UnknownError.Wrap(err)
		}
		if delivered > st.Delivered {
			released++
		}
		b.staging.Release(st.ID, delivered)
	}

	// Staging is now as of Q, and says so: a reader takes staging and the
	// block it is as of together, and one that paired this stage with any
	// other block would execute a different block (#4291).
	b.staging.AtBlock(q)
	b.staging.Commit()

	// This node executed no block at or below Q since it started, so a
	// Directory receipt for one of those blocks it does not hold is not a
	// miss (#4294). Which of them its store holds the synthetics of is the
	// store's to say, and the seed reads it there (#4400).
	x.synthCache().JoinedAt(q)
	x.logger.Info("Staging settled at the block the state is",
		"module", "sync", "partition", x.Describe.PartitionId, "block", q,
		"streams", len(streams), "released", released, "directoryAnchorBlock", through)
	return nil
}

// deliveredFrom is what a stream's ledger says it has delivered from its
// source — the ledger, never staging's memory of it: staging's copy is zero
// on a node that has not executed a block, and the ledger is what the peers
// wrote (#4291's trap, and invariant 1).
func deliveredFrom(batch *database.Batch, id execute.StreamID) (uint64, error) {
	var ledger protocol.SequenceLedger
	switch err := batch.Account(id.Ledger).Main().GetAs(&ledger); {
	case errors.Is(err, errors.NotFound):
		return 0, nil
	case err != nil:
		return 0, errors.UnknownError.WithFormat("load %v: %w", id.Ledger, err)
	}
	return ledger.Partition(id.Source).Delivered, nil
}
