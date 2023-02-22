// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (x *Executor) ProduceSynthetic(batch *database.Batch, produced []*ProducedMessage) error {
	if len(produced) == 0 {
		return nil
	}

	batch = batch.Begin(true)
	defer batch.Discard()

	// Collect transactions and refundable synthetic transactions
	txns := map[[32]byte]*protocol.Transaction{}
	swos := map[[32]byte][]protocol.SynthTxnWithOrigin{}
	for _, p := range produced {
		from, err := batch.Message(p.Producer.Hash()).Main().Get()
		if err != nil {
			return errors.InternalError.WithFormat("load message: %w", err)
		}
		fromTxn, ok := from.(messaging.MessageWithTransaction)
		if !ok {
			continue
		}

		sub, ok := p.Message.(messaging.MessageWithTransaction)
		if !ok {
			continue
		}

		swo, ok := sub.GetTransaction().Body.(protocol.SynthTxnWithOrigin)
		if !ok {
			continue
		}

		swo.SetCause(fromTxn.ID().Hash(), fromTxn.ID().Account())

		h := p.Producer.Hash()
		txns[h] = fromTxn.GetTransaction()
		swos[h] = append(swos[h], swo)
	}

	// Calculate refund amounts
	for hash, from := range txns {
		err := x.setSyntheticOrigin(batch, from, swos[hash])
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	// Shouldn't this be recorded somewhere?
	state := new(chain.ChainUpdates)

	// Finalize the produced transactions
	for _, p := range produced {
		seq, err := x.buildSynthTxn(state, batch, p)
		if err != nil {
			return err
		}

		// Record message -> produced synthetic message
		h := p.Producer.Hash()
		err = batch.Message(h).Produced().Add(seq.Message.ID())
		if err != nil {
			return errors.UnknownError.WithFormat("add produced: %w", err)
		}

		err = batch.Transaction(h[:]).Produced().Add(seq.Message.ID())
		if err != nil {
			return errors.UnknownError.WithFormat("add produced: %w", err)
		}
	}

	err := batch.Commit()
	if err != nil {
		return errors.UnknownError.WithFormat("commit batch: %w", err)
	}

	return nil
}

// setSyntheticOrigin sets the synthetic origin data of the synthetic
// transaction. setSyntheticOrigin sets the refund amount for each synthetic
// transaction, spreading the potential refund across all produced synthetic
// transactions.
func (x *Executor) setSyntheticOrigin(batch *database.Batch, from *protocol.Transaction, produced []protocol.SynthTxnWithOrigin) error {
	// Set the refund amount for each output
	refund, err := x.globals.Active.Globals.FeeSchedule.ComputeSyntheticRefund(from, len(produced))
	if err != nil {
		return errors.InternalError.WithFormat("compute refund: %w", err)
	}

	status, err := batch.Transaction(from.GetHash()).GetStatus()
	if err != nil {
		return errors.UnknownError.WithFormat("load status: %w", err)
	}

	for _, swo := range produced {
		swo.SetRefund(status.Initiator, refund)
	}
	return nil
}

func (m *Executor) buildSynthTxn(state *chain.ChainUpdates, batch *database.Batch, prod *ProducedMessage) (*messaging.SequencedMessage, error) {
	// Generate a synthetic tx and send to the router. Need to track txid to
	// make sure they get processed.

	var ledger *protocol.SyntheticLedger
	record := batch.Account(m.Describe.Synthetic())
	err := record.GetStateAs(&ledger)
	if err != nil {
		// If we can't load the ledger, the node is fubared
		panic(fmt.Errorf("failed to load the ledger: %v", err))
	}

	destPart, err := m.Router.RouteAccount(prod.Destination)
	if err != nil {
		return nil, err
	}
	destPartUrl := protocol.PartitionUrl(destPart)
	destLedger := ledger.Partition(destPartUrl)
	destLedger.Produced++

	seq := new(messaging.SequencedMessage)
	seq.Message = prod.Message
	seq.Source = m.Describe.NodeUrl()
	seq.Destination = destPartUrl
	seq.Number = destLedger.Produced
	m.logger.Debug("Built synthetic transaction", "module", "synthetic", "txid", logging.AsHex(prod.Message.Hash()), "destination", prod.Destination, "seq-num", destLedger.Produced)

	// Update the ledger
	err = record.PutState(ledger)
	if err != nil {
		return nil, err
	}

	// Store the transaction, its status, and the initiator
	err = putMessageWithStatus(
		batch, seq,
		&protocol.TransactionStatus{
			Code: errors.Remote,
		})
	if err != nil {
		return nil, err
	}

	// Add the transaction to the synthetic transaction chain
	chain, err := record.MainChain().Get()
	if err != nil {
		return nil, err
	}

	h := seq.Hash()
	index := chain.Height()
	err = chain.AddEntry(h[:], false)
	if err != nil {
		return nil, err
	}

	err = state.DidAddChainEntry(batch, m.Describe.Synthetic(), protocol.MainChain, protocol.ChainTypeTransaction, h[:], uint64(index), 0, 0)
	if err != nil {
		return nil, err
	}

	partition, ok := protocol.ParsePartitionUrl(seq.Destination)
	if !ok {
		return nil, errors.InternalError.WithFormat("destination URL is not a valid partition")
	}

	indexIndex, err := addIndexChainEntry(record.SyntheticSequenceChain(partition), &protocol.IndexEntry{
		Source: uint64(index),
	})
	if err != nil {
		return nil, err
	}
	if indexIndex+1 != seq.Number {
		m.logger.Error("Sequence number does not match index chain index", "seq-num", seq.Number, "index", indexIndex, "source", seq.Source, "destination", seq.Destination)
	}

	return seq, nil
}

func putMessageWithStatus(batch *database.Batch, message messaging.Message, status *protocol.TransactionStatus) error {
	// Store the transaction
	h := message.Hash()
	err := batch.Message(h).Main().Put(message)
	if err != nil {
		return fmt.Errorf("store transaction: %w", err)
	}

	// Update the status
	err = batch.Transaction(h[:]).PutStatus(status)
	if err != nil {
		return fmt.Errorf("store status: %w", err)
	}

	return nil
}
