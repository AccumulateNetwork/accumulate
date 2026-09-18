// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"sort"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
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
		numbers := make([]uint64, 0, len(c.arrivals[k]))
		for n := range c.arrivals[k] {
			numbers = append(numbers, n)
		}
		sort.Slice(numbers, func(i, j int) bool { return numbers[i] < numbers[j] })
		for _, n := range numbers {
			held, err := b.collectArrival(str, pos.delivered, c.arrivals[k][n])
			if err != nil {
				b.staging.Discard()
				return nil, errors.UnknownError.Wrap(err)
			}
			if held {
				out.Held++
			}
		}
	}

	out.Accounts = accountsNamed(envelopes)
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
// hold it, and answers whether it was held.
//
// The form matters as much as the fact. An entry that passes its proof is
// held by the sequenced layer as the sequenced message itself, not collected,
// and runs the moment its number is next; an entry that does not is held as
// collection holds it — the outer message, marked collected, with the hash a
// proof must validate, and it never runs until one does (executor spec,
// "Collection"). Holding the wrong one either strands an entry its peers ran
// or runs one its peers did not.
func (b *Block) collectArrival(str stream, delivered uint64, a *arrival) (bool, error) {
	if a.seq.Number <= delivered || a.seq.Number > delivered+maxSequenceAhead {
		// At or below what this node's store says it delivered, or past the
		// sanity horizon. Nothing below Delivered can ever run again, and
		// the pulled state decides the rest at Q.
		return false, nil
	}
	if _, already := b.staging.IDOf(str.id(), a.seq.Number); already {
		return false, nil // first sighting wins, here as in a block
	}

	if str.kind == streamAnchor {
		// An anchor copy, held as BlockAnchor.process holds one below its
		// quorum: the sequenced message, and the hash a proof over the
		// source's anchor chain validates — the transaction's stored form,
		// without a principal. Its signatures are in the store, which the
		// pull fills; the quorum is decided when it runs.
		txn, ok := a.seq.Message.(*messaging.TransactionMessage)
		if !ok || txn.Transaction == nil {
			return false, nil
		}
		stored := new(protocol.Transaction)
		stored.Body = txn.Transaction.Body
		b.staging.Hold(str.id(), a.seq.Number, &execute.Held{
			ID:        a.seq.ID(),
			Message:   a.seq,
			Collected: true,
			Hash:      *(*[32]byte)(stored.GetHash()),
		})
		return true, nil
	}

	// A source whose proof this block turned away for want of budget has its
	// entries turned away with it, as collection does: nothing re-sends a
	// proof, so holding the entry would strand it where no gap is left for
	// healing to find (#4282).
	if b.proofBudgetBound[strings.ToLower(str.source.String())] {
		return false, nil
	}

	ok, err := b.admissibilityOf(str, a.classifier, a.seq)
	if err != nil {
		// A message whose admissibility cannot be decided is skipped, as
		// stageRuns skips it; the number stays a hole and is asked for.
		return false, nil
	}
	if ok {
		b.staging.Hold(str.id(), a.seq.Number, &execute.Held{ID: a.seq.ID(), Message: a.seq})
		return true, nil
	}

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
	return true, nil
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
	if ledger.Index != q {
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

	b.staging.Commit()
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

// accountsNamed is every account a block's envelopes name: what the state
// pull must have for the block to execute (#4293). A transaction names its
// principal, a signature names its signer and the transaction it signs, a
// sequenced message names the principal of what it carries. Nothing here
// decides anything; a name too many costs one pull.
func accountsNamed(envelopes []*messaging.Envelope) []*url.URL {
	seen := map[string]*url.URL{}
	add := func(u *url.URL) {
		if u == nil {
			return
		}
		k := strings.ToLower(u.String())
		if _, ok := seen[k]; !ok {
			seen[k] = u
		}
	}
	var addTxn func(txn *protocol.Transaction)
	addTxn = func(txn *protocol.Transaction) {
		if txn == nil {
			return
		}
		add(txn.Header.Principal)
	}
	var addMsg func(msg messaging.Message)
	addMsg = func(msg messaging.Message) {
		switch m := msg.(type) {
		case *messaging.TransactionMessage:
			addTxn(m.Transaction)
		case *messaging.SignatureMessage:
			if m.TxID != nil {
				add(m.TxID.Account())
			}
			if sig, ok := m.Signature.(protocol.Signature); ok {
				add(sig.RoutingLocation())
			}
		case *messaging.SequencedMessage:
			addMsg(m.Message)
		case *messaging.SyntheticMessage:
			addMsg(m.Message)
		case *messaging.BadSyntheticMessage:
			addMsg(m.Message)
		case *messaging.BlockAnchor:
			addMsg(m.Anchor)
		}
	}
	for _, env := range envelopes {
		messages, err := env.Normalize()
		if err != nil {
			continue // a malformed envelope names nothing
		}
		for _, msg := range messages {
			addMsg(msg)
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]*url.URL, 0, len(keys))
	for _, k := range keys {
		out = append(out, seen[k])
	}
	return out
}
