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

	// The synthetic chain's state before this block's first element: what a
	// proof over the block's entries is built from, captured once, from the
	// head the block already holds.
	synthChain := batch.Account(x.Describe.Synthetic()).MainChain().Inner()
	head, err := synthChain.Head().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load synthetic chain head: %w", err)
	}
	blk := &synthcache.Block{
		Index:   block,
		Segment: &merkle.Segment{First: head.Count, Before: head.Copy(), MarkMask: synthChain.MarkMask()},
	}

	// Finalize the produced transactions
	for _, p := range produced {
		seq, index, err := x.buildSynthTxn(state, batch, p, block)
		if err != nil {
			return nil, err
		}

		h := seq.Hash()
		blk.Segment.Append(h[:])
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

		if p.Producer == nil {
			continue
		}

		// Record message -> produced synthetic message
		ph := p.Producer.Hash()
		err = batch.Message(ph).Produced().Add(seq.Message.ID())
		if err != nil {
			return nil, errors.UnknownError.WithFormat("add produced: %w", err)
		}

		err = batch.Transaction(ph[:]).Produced().Add(seq.Message.ID())
		if err != nil {
			return nil, errors.UnknownError.WithFormat("add produced: %w", err)
		}
	}

	err = batch.Commit()
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

	// Are the body or header exactly 64 bytes?
	body, err := txn.Transaction.Body.MarshalBinary()
	if err != nil {
		return errors.EncodingError.WithFormat("marshal body: %w", err)
	}
	header, err := txn.Transaction.Header.MarshalBinary()
	if err != nil {
		return errors.EncodingError.WithFormat("marshal header: %w", err)
	}
	if len(body) != 64 && len(header) != 64 {
		return nil
	}

	// Copy to reset the cached hash if there is one
	txn = txn.Copy()

	// Pad the header and/or body
	if len(body) == 64 {
		body = append(body, 0)
		txn.Transaction.Body, err = protocol.UnmarshalTransactionBody(body)
		if err != nil {
			return errors.EncodingError.WithFormat("unmarshal body: %w", err)
		}
	}
	if len(header) == 64 {
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

func (m *Executor) buildSynthTxn(state *chain.ChainUpdates, batch *database.Batch, prod *ProducedMessage, block uint64) (*messaging.SequencedMessage, int64, error) {
	// Generate a synthetic tx and send to the router. Need to track txid to
	// make sure they get processed.

	err := adjust64(prod)
	if err != nil {
		return nil, 0, errors.UnknownError.WithFormat("pad synthetic message: %w", err)
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
		return nil, 0, err
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
		return nil, 0, err
	}

	// Store the transaction, its status, and the initiator
	err = putMessageWithStatus(
		batch, seq,
		&protocol.TransactionStatus{
			Code: errors.Remote,
		})
	if err != nil {
		return nil, 0, err
	}

	// Add the transaction to the synthetic transaction chain
	chain, err := record.MainChain().Get()
	if err != nil {
		return nil, 0, err
	}

	h := seq.Hash()
	index := chain.Height()
	err = chain.AddEntry(h[:], false)
	if err != nil {
		return nil, 0, err
	}

	err = state.DidAddChainEntry(batch, m.Describe.Synthetic(), protocol.MainChain, protocol.ChainTypeTransaction, h[:], uint64(index), 0, 0)
	if err != nil {
		return nil, 0, err
	}

	partition, ok := protocol.ParsePartitionUrl(seq.Destination)
	if !ok {
		return nil, 0, errors.InternalError.WithFormat("destination URL is not a valid partition")
	}

	indexIndex, err := addIndexChainEntry(record.SyntheticSequenceChain(partition), &protocol.IndexEntry{
		Source: uint64(index),
	})
	if err != nil {
		return nil, 0, err
	}
	if indexIndex+1 != seq.Number {
		m.logger.Error("Sequence number does not match index chain index", "seq-num", seq.Number, "index", indexIndex, "source", seq.Source, "destination", seq.Destination)
	}

	return seq, index, nil
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
