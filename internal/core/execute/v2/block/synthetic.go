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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
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

	for hash, from := range txns {
		err := x.setSyntheticOrigin(batch, from, swos[hash])
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	// Finalize the produced transactions
	state := new(chain.ChainUpdates)
	for _, p := range produced {
		from, err := batch.Message(p.Producer.Hash()).Main().Get()
		if err != nil {
			return errors.InternalError.WithFormat("load message: %w", err)
		}
		fromTxn, _ := from.(messaging.MessageWithTransaction)

		sub, ok := p.Message.(messaging.MessageWithTransaction)
		if !ok {
			return errors.InternalError.WithFormat("invalid produced message type %v", p.Message.Type())
		}

		tx, err := x.buildSynthTxn(state, batch, sub.GetTransaction().Header.Principal, sub.GetTransaction().Body)
		if err != nil {
			return err
		}
		sub.GetTransaction().Header = tx.Header

		// Don't record txn -> produced synth txn for internal transactions
		if fromTxn != nil && fromTxn.GetTransaction().Body.Type().IsSystem() {
			continue
		}

		switch body := sub.GetTransaction().Body.(type) {
		case protocol.SynthTxnWithOrigin:
			// Record transaction -> produced synthetic transaction
			h := from.ID().Hash()
			err = batch.Transaction(h[:]).Produced().Add(tx.ID())
			if err != nil {
				return err
			}

		case *protocol.SyntheticForwardTransaction:
			// Record signature -> produced synthetic forwarded transaction, forwarded signature
			for _, sig := range body.Signatures {
				sigId := sig.Destination.WithTxID(*(*[32]byte)(sig.Signature.Hash()))
				for _, hash := range sig.Cause {
					err = batch.Transaction(hash[:]).Produced().Add(tx.ID(), sigId) //nolint:rangevarref
					if err != nil {
						return err
					}
				}
			}
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

func (m *Executor) buildSynthTxn(state *chain.ChainUpdates, batch *database.Batch, dest *url.URL, body protocol.TransactionBody) (*protocol.Transaction, error) {
	// Generate a synthetic tx and send to the router. Need to track txid to
	// make sure they get processed.

	var ledger *protocol.SyntheticLedger
	record := batch.Account(m.Describe.Synthetic())
	err := record.GetStateAs(&ledger)
	if err != nil {
		// If we can't load the ledger, the node is fubared
		panic(fmt.Errorf("failed to load the ledger: %v", err))
	}

	destPart, err := m.Router.RouteAccount(dest)
	if err != nil {
		return nil, err
	}
	destPartUrl := protocol.PartitionUrl(destPart)
	destLedger := ledger.Partition(destPartUrl)
	destLedger.Produced++

	txn := new(protocol.Transaction)
	txn.Header.Principal = dest
	txn.Header.Source = m.Describe.NodeUrl()
	txn.Header.Destination = destPartUrl
	txn.Header.SequenceNumber = destLedger.Produced
	txn.Body = body
	m.logger.Debug("Built synthetic transaction", "module", "synthetic", "txid", logging.AsHex(txn.GetHash()), "destination", dest.String(), "type", body.Type(), "seq-num", destLedger.Produced)

	// Update the ledger
	err = record.PutState(ledger)
	if err != nil {
		return nil, err
	}

	// Store the transaction, its status, and the initiator
	err = putSyntheticTransaction(
		batch, txn,
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

	index := chain.Height()
	err = chain.AddEntry(txn.GetHash(), false)
	if err != nil {
		return nil, err
	}

	err = state.DidAddChainEntry(batch, m.Describe.Synthetic(), protocol.MainChain, protocol.ChainTypeTransaction, txn.GetHash(), uint64(index), 0, 0)
	if err != nil {
		return nil, err
	}

	partition, ok := protocol.ParsePartitionUrl(txn.Header.Destination)
	if !ok {
		return nil, errors.InternalError.WithFormat("destination URL is not a valid partition")
	}

	indexIndex, err := addIndexChainEntry(record.SyntheticSequenceChain(partition), &protocol.IndexEntry{
		Source: uint64(index),
	})
	if err != nil {
		return nil, err
	}
	if indexIndex+1 != txn.Header.SequenceNumber {
		m.logger.Error("Sequence number does not match index chain index", "seq-num", txn.Header.SequenceNumber, "index", indexIndex, "source", txn.Header.Source, "destination", txn.Header.Destination)
	}

	return txn, nil
}

func processSyntheticTransaction(batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) error {
	// Check for a key signature
	hasKeySig, err := hasKeySignature(batch, status)
	if err != nil {
		return err
	}
	if !hasKeySig {
		return errors.Unauthenticated.WithFormat("missing key signature")
	}

	if transaction.Body.Type() == protocol.TransactionTypeDirectoryAnchor || transaction.Body.Type() == protocol.TransactionTypeBlockValidatorAnchor {
		return nil
	}

	return nil
}

func putSyntheticTransaction(batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) error {
	// Store the transaction
	err := batch.Message(transaction.ID().Hash()).Main().Put(&messaging.UserTransaction{Transaction: transaction})
	if err != nil {
		return fmt.Errorf("store transaction: %w", err)
	}

	// Update the status
	err = batch.Transaction(transaction.GetHash()).PutStatus(status)
	if err != nil {
		return fmt.Errorf("store status: %w", err)
	}

	return nil
}
