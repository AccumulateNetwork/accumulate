// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Collect takes healed packages into staging outside any block. It is what a
// node does when it rejoins after a restart: staging is memory and is gone,
// the ledger says what each stream has delivered, and the sources' caches
// hold what the node's peers still hold above that. The peers will execute
// those entries the moment their proof's anchor lands, so the node must hold
// them before it executes that block or it executes a different block
// (executor spec, "Sync"; healing spec, "Rejoin"; #4290).
//
// Each envelope is a package as the requester builds it -- one proof and the
// synthetic entries it covers, or block anchor copies -- and goes through the
// intake a block gives a package: the proof to anchor staging, or decided at
// once against the anchors already executed; each entry held at its number.
// An anchor is held as the block holds a copy below its quorum: the
// sequenced message, runnable once the signatures recorded in the store
// reach the threshold. Nothing is written to the store. Returns how many
// entries were held.
func (x *Executor) Collect(batch *database.Batch, envelopes []*messaging.Envelope) (int, error) {
	b := &Block{Executor: x, Batch: batch, staging: x.staging().Begin()}
	held := 0
	for _, env := range envelopes {
		n, err := b.collectPackage(env)
		if err != nil {
			b.staging.Discard()
			return 0, err
		}
		held += n
	}
	b.staging.Commit()
	return held, nil
}

// member is one sequenced message of a package, with its stream.
type member struct {
	msg messaging.Message
	seq *messaging.SequencedMessage
	str stream
}

// collectPackage is one package's intake. A package whose proof is refused is
// not held at all: its entries would wait for a proof that never comes, and
// the hole is asked for again instead.
func (b *Block) collectPackage(env *messaging.Envelope) (int, error) {
	messages, err := env.Normalize()
	if err != nil {
		return 0, errors.BadRequest.WithFormat("normalize: %w", err)
	}
	var proofs []*protocol.AnnotatedReceipt
	var source *url.URL
	var siblings [][]byte
	var members []member
	resolve := resolveFromBatch(b.Batch)
	var anchors []member
	for _, msg := range messages {
		if p, ok := msg.(*messaging.SyntheticProof); ok && p.Proof != nil {
			proofs = append(proofs, p.Proof)
			continue
		}
		str, seq, err := b.Executor.streamOf(msg, resolve)
		if err != nil || !str.ok() {
			continue
		}
		if str.kind == streamAnchor {
			anchors = append(anchors, member{msg, seq, str})
			continue
		}
		if source == nil {
			source = str.source
		}
		if !source.Equal(str.source) {
			continue
		}
		h := seq.Hash()
		siblings = append(siblings, h[:])
		members = append(members, member{msg, seq, str})
	}
	if len(anchors) > 0 {
		return b.holdAnchors(anchors)
	}
	if len(members) == 0 || len(proofs) == 0 {
		return 0, nil
	}
	for _, p := range proofs {
		switch err := b.intakeProof(source, p, siblings); {
		case err == nil:
		case errors.Is(err, errors.BadRequest):
			return 0, nil // refused, and counted by the intake
		default:
			return 0, err
		}
	}

	var delivered uint64
	var ledger protocol.SequenceLedger
	switch err := b.Batch.Account(members[0].str.ledger).Main().GetAs(&ledger); {
	case errors.Is(err, errors.NotFound):
	case err != nil:
		return 0, errors.UnknownError.WithFormat("load %v: %w", members[0].str.ledger, err)
	default:
		delivered = ledger.Partition(source).Delivered
	}
	held := 0
	for _, m := range members {
		if m.seq.Number <= delivered || m.seq.Number > delivered+maxSequenceAhead {
			continue
		}
		h := &execute.Held{ID: m.msg.ID(), Message: m.msg, Collected: true, Hash: m.seq.Hash()}
		if mt, ok := m.seq.Message.(messaging.MessageForTransaction); ok {
			want := mt.GetTxID().Hash()
			for _, sibling := range messages {
				txn, ok := sibling.(messaging.MessageWithTransaction)
				if ok && txn.GetTransaction().ID().Hash() == want {
					h.Companion = sibling
					break
				}
			}
		}
		b.staging.Hold(m.str.id(), m.seq.Number, h)
		held++
	}
	return held, nil
}

// holdAnchors holds anchor copies above the anchor ledger's Delivered, one
// entry per number, as BlockAnchor.process holds a copy below its quorum: the
// sequenced message, and the hash a proof over the source's anchor chain
// validates (the transaction's stored form, without a principal).
func (b *Block) holdAnchors(anchors []member) (int, error) {
	delivered := map[string]uint64{}
	held := 0
	for _, m := range anchors {
		k := m.str.key()
		d, ok := delivered[k]
		if !ok {
			var ledger protocol.SequenceLedger
			switch err := b.Batch.Account(m.str.ledger).Main().GetAs(&ledger); {
			case errors.Is(err, errors.NotFound):
			case err != nil:
				return 0, errors.UnknownError.WithFormat("load %v: %w", m.str.ledger, err)
			default:
				d = ledger.Partition(m.str.source).Delivered
			}
			delivered[k] = d
		}
		if m.seq.Number <= d || m.seq.Number > d+maxSequenceAhead {
			continue
		}
		txn, ok := m.seq.Message.(*messaging.TransactionMessage)
		if !ok || txn.Transaction == nil {
			continue
		}
		stored := new(protocol.Transaction)
		stored.Body = txn.Transaction.Body
		if _, already := b.staging.IDOf(m.str.id(), m.seq.Number); already {
			continue
		}
		b.staging.Hold(m.str.id(), m.seq.Number, &execute.Held{
			ID:        m.seq.ID(),
			Message:   m.seq,
			Collected: true,
			Hash:      *(*[32]byte)(stored.GetHash()),
		})
		held++
	}
	return held, nil
}
