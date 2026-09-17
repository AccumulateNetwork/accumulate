// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// compareProduced is the canonical order for sequencing produced messages
// (#4144): by producer transaction ID, with producer-less (system-produced)
// messages after every attributed one, keyed among themselves by message
// hash. The key is derivable from the produced set alone — never from
// delivery order, which becomes shard-scheduling-dependent under parallel
// execution (#4145). Ties (same producer) return 0 so that a STABLE sort
// preserves the producer's emission order; that order is the second half of
// the (producer, index-within-producer) key.
func compareProduced(a, b *ProducedMessage) int {
	switch {
	case a.Producer == nil && b.Producer == nil:
		ah, bh := a.Message.Hash(), b.Message.Hash()
		if c := bytes.Compare(ah[:], bh[:]); c != 0 {
			return c
		}
		// DISTINCT system productions can share a hash — MakeMajorBlock is
		// byte-identical for every BVN — and insertion order must not be
		// load-bearing (#4149). Destination breaks the tie.
		return a.Destination.Compare(b.Destination)
	case a.Producer == nil:
		return +1 // system-produced messages order after attributed ones
	case b.Producer == nil:
		return -1
	default:
		return a.Producer.Compare(b.Producer)
	}
}

// sortProduced sorts produced messages into the canonical sequencing order.
// The sort must be stable: compareProduced deliberately reports equality for
// messages of the same producer, so their emitted order survives.
func sortProduced(produced []*ProducedMessage) {
	sort.SliceStable(produced, func(i, j int) bool {
		return compareProduced(produced[i], produced[j]) < 0
	})
}

func (x *Executor) produceSynthetic(batch *database.Batch, produced []*ProducedMessage, block uint64) error {
	_, err := x.produceSyntheticInto(batch, produced, block, nil)
	return err
}

// produceSyntheticInto sequences the block's produced messages onto the
// synthetic chain and records each in the producer's cache as it is
// produced: the entry itself, and the block's segment of the synthetic chain
// that its proofs are built from (healing spec, "The cache"). The returned
// block is completed with the root receipt at close.
func (x *Executor) produceSyntheticInto(batch *database.Batch, produced []*ProducedMessage, block uint64, tx *synthcache.Txn) (*synthcache.Block, error) {
	if len(produced) == 0 {
		return nil, nil
	}

	batch = batch.Begin(true)
	defer batch.Discard()

	// Shouldn't this be recorded somewhere?
	state := new(chain.ChainUpdates)

	blk := &synthcache.Block{Index: block, Streams: map[string]*synthcache.Stream{}}

	// Finalize the produced transactions
	for _, p := range produced {
		seq, index, before, err := x.buildSynthTxn(state, batch, p, block)
		if err != nil {
			return nil, err
		}

		h := seq.Hash()
		// One segment per destination: the chain's state before this block's
		// first entry for it, then the entries in order (executor spec, "One
		// chain per pair, one stage per chain"). A proof over the segment is
		// exactly the destination's entries, and its index is the sequence
		// number less one.
		st := blk.Stream(seq.Destination)
		if st == nil {
			partition, _ := protocol.ParsePartitionUrl(seq.Destination)
			c := batch.Account(x.Describe.Synthetic()).SyntheticChain(partition)
			st = &synthcache.Stream{Destination: seq.Destination, ChainName: c.Name(), Segment: &merkle.Segment{First: index, Before: before, MarkMask: c.Inner().MarkMask()}}
			blk.Streams[synthcache.StreamKey(seq.Destination)] = st
		}
		st.Segment.Append(h[:])
		entry := &synthcache.Entry{Stream: seq.Destination, Number: seq.Number, Index: index, Block: block, Hash: h, Seq: seq}
		// The transaction the message belongs to travels with it. It
		// executed in this block or is pending here, so it is recent state.
		if msg, ok := seq.Message.(messaging.MessageForTransaction); ok && seq.Message.Type() != messaging.MessageTypeBlockAnchor {
			var txn messaging.MessageWithTransaction
			err := batch.Message(msg.GetTxID().Hash()).Main().GetAs(&txn)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("load transaction for synthetic message: %w", err)
			}
			entry.Companion = txn
		}
		blk.Entries = append(blk.Entries, entry)
		tx.Add(entry)
		// The producer's Produced set was written when the message was
		// produced (didProduce), with the ID it carries here.
	}

	err := batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("commit batch: %w", err)
	}

	return blk, nil
}

// setSyntheticOrigin sets the synthetic origin data of the synthetic
// transaction. setSyntheticOrigin sets the refund amount for each synthetic
// transaction, spreading the potential refund across all produced synthetic
// transactions.
func (x *Executor) setSyntheticOrigin(batch *database.Batch, from *protocol.Transaction, produced []protocol.SyntheticTransaction) error {
	for _, swo := range produced {
		swo.SetCause(from.ID().Hash(), from.ID().Account())
	}

	if !from.Body.Type().IsUser() {
		return nil
	}

	// Set the refund amount for each output
	refund, err := x.globals().Active.Globals.FeeSchedule.ComputeSyntheticRefund(from, len(produced))
	if err != nil {
		return errors.InternalError.WithFormat("compute refund: %w", err)
	}

	isInit, initiator, err := transactionIsInitiated(batch, from.ID())
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if !isInit {
		return errors.InternalError.WithFormat("producer is not initiated")
	}

	for _, swo := range produced {
		swo.SetCause(from.ID().Hash(), from.ID().Account())
		swo.SetRefund(initiator.Payer, refund)
	}
	return nil
}

func adjust64(prod *ProducedMessage) error {
	txn, ok := prod.Message.(*messaging.TransactionMessage)
	if !ok {
		return nil
	}

	// Are the body or header exactly 64 bytes? The transaction's hash
	// computation already measures both, and the hash is needed anyway, so
	// nothing is marshaled to find out (#4245).
	body64, header64 := txn.Transaction.BodyIs64Bytes(), txn.Transaction.HeaderIs64Bytes()
	if !body64 && !header64 {
		return nil
	}

	// Copy to reset the cached hash if there is one
	txn = txn.Copy()

	// Pad the header and/or body
	if body64 {
		body, err := txn.Transaction.Body.MarshalBinary()
		if err != nil {
			return errors.EncodingError.WithFormat("marshal body: %w", err)
		}
		body = append(body, 0)
		txn.Transaction.Body, err = protocol.UnmarshalTransactionBody(body)
		if err != nil {
			return errors.EncodingError.WithFormat("unmarshal body: %w", err)
		}
	}
	if header64 {
		header, err := txn.Transaction.Header.MarshalBinary()
		if err != nil {
			return errors.EncodingError.WithFormat("marshal header: %w", err)
		}
		header = append(header, 0)
		txn.Transaction.Header = protocol.TransactionHeader{}
		err = txn.Transaction.Header.UnmarshalBinary(header)
		if err != nil {
			return errors.EncodingError.WithFormat("unmarshal header: %w", err)
		}
	}

	prod.Message = txn
	return nil
}

func (m *Executor) buildSynthTxn(state *chain.ChainUpdates, batch *database.Batch, prod *ProducedMessage, block uint64) (*messaging.SequencedMessage, int64, *merkle.State, error) {
	// Generate a synthetic tx and send to the router. Need to track txid to
	// make sure they get processed.

	err := adjust64(prod)
	if err != nil {
		return nil, 0, nil, errors.UnknownError.WithFormat("pad synthetic message: %w", err)
	}

	var ledger *protocol.SyntheticLedger
	record := batch.Account(m.Describe.Synthetic())
	err = record.Main().GetAs(&ledger)
	if err != nil {
		// If we can't load the ledger, the node is fubared
		panic(fmt.Errorf("failed to load the ledger: %v", err))
	}

	destPart, err := m.Router.RouteAccount(prod.Destination)
	if err != nil {
		return nil, 0, nil, err
	}
	destPartUrl := protocol.PartitionUrl(destPart)
	destLedger := ledger.Partition(destPartUrl)
	destLedger.Produced++

	seq := new(messaging.SequencedMessage)
	seq.Message = prod.Message
	seq.Source = m.Describe.NodeUrl()
	seq.Destination = destPartUrl
	seq.Number = destLedger.Produced
	m.logger.Debug("Built synthetic transaction", "module", "synthetic",
		"block", block,
		"txid", logging.AsHex(prod.Message.Hash()),
		"destination", prod.Destination,
		"seq-num", destLedger.Produced,
		"type", seq.Message.Type())

	// Update the ledger
	err = record.Main().Put(ledger)
	if err != nil {
		return nil, 0, nil, err
	}

	// Store the sequenced message: the chain entry names it, and the
	// sequencer serves it from here (H1). No status — the outcome is the
	// destination's, and nothing here reads one (executor spec, "One status
	// per outcome").
	err = batch.Message(seq.Hash()).Main().Put(seq)
	if err != nil {
		return nil, 0, nil, errors.UnknownError.WithFormat("store sequenced message: %w", err)
	}

	// Add the transaction to the destination's own synthetic chain: entry n-1
	// is sequence number n, so a proof over a span of the chain is exactly the
	// destination's entries in order (executor spec, "One chain per pair, one
	// stage per chain").
	chain2 := record.SyntheticChain(destPart)
	before, err := chain2.Head().Get()
	if err != nil {
		return nil, 0, nil, errors.UnknownError.WithFormat("load synthetic chain head: %w", err)
	}
	before = before.Copy()
	chain, err := chain2.Get()
	if err != nil {
		return nil, 0, nil, err
	}

	h := seq.Hash()
	index := chain.Height()
	if uint64(index)+1 != seq.Number {
		return nil, 0, nil, errors.InternalError.WithFormat("synthetic chain to %v is at %d but the next sequence number is %d", seq.Destination, index, seq.Number)
	}
	err = chain.AddEntry(h[:], false)
	if err != nil {
		return nil, 0, nil, err
	}

	err = state.DidAddChainEntry(batch, m.Describe.Synthetic(), chain2.Name(), protocol.ChainTypeTransaction, h[:], uint64(index), 0, 0)
	if err != nil {
		return nil, 0, nil, err
	}

	return seq, index, before, nil
}

func putMessageWithStatus(batch *database.Batch, message messaging.Message, status *protocol.TransactionStatus) error {
	// Store the transaction
	h := message.Hash()
	err := batch.Message(h).Main().Put(message)
	if err != nil {
		return fmt.Errorf("store transaction: %w", err)
	}

	// Update the status
	err = batch.Transaction(h[:]).Status().Put(status)
	if err != nil {
		return fmt.Errorf("store status: %w", err)
	}

	return nil
}
