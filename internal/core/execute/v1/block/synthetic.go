// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (x *Executor) ProduceSynthetic(batch *database.Batch, from *protocol.Transaction, produced []*protocol.Transaction) error {
	if len(produced) == 0 {
		return nil
	}

	err := x.setSyntheticOrigin(batch, from, produced)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	state := new(chain.ChainUpdates)
	for _, sub := range produced {
		tx, err := x.buildSynthTxn(state, batch, sub.Header.Principal, sub.Body)
		if err != nil {
			return err
		}
		sub.Header = tx.Header

		// Don't record txn -> produced synth txn for internal transactions
		if from.Body.Type().IsSystem() {
			continue
		}

		switch body := sub.Body.(type) {
		case protocol.SynthTxnWithOrigin:
			// Record transaction -> produced synthetic transaction
			err = batch.Transaction(from.GetHash()).Produced().Add(tx.ID())
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

	return nil
}

// setSyntheticOrigin sets the synthetic origin data of the synthetic
// transaction. setSyntheticOrigin sets the refund amount for each synthetic
// transaction, spreading the potential refund across all produced synthetic
// transactions.
func (x *Executor) setSyntheticOrigin(batch *database.Batch, from *protocol.Transaction, produced []*protocol.Transaction) error {
	if len(produced) == 0 {
		return nil
	}

	// Find all the synthetic transactions that implement the interface
	var swos []protocol.SynthTxnWithOrigin
	for _, txn := range produced {
		swo, ok := txn.Body.(protocol.SynthTxnWithOrigin)
		if !ok {
			continue
		}

		swos = append(swos, swo)
		swo.SetCause(*(*[32]byte)(from.GetHash()), from.Header.Principal)
	}
	if len(swos) == 0 {
		return nil
	}

	// Set the refund amount for each output
	refund, err := x.globals.Active.Globals.FeeSchedule.ComputeSyntheticRefund(from, len(swos))
	if err != nil {
		return errors.InternalError.WithFormat("compute refund: %w", err)
	}

	status, err := batch.Transaction(from.GetHash()).Status().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load status: %w", err)
	}

	for _, swo := range swos {
		swo.SetRefund(status.Initiator, refund)
	}
	return nil
}

func (m *Executor) buildSynthTxn(state *chain.ChainUpdates, batch *database.Batch, dest *url.URL, body protocol.TransactionBody) (*protocol.Transaction, error) {
	// Generate a synthetic tx and send to the router. Need to track txid to
	// make sure they get processed.

	// Add padding if the body is 64 bytes
	if m.globals.Active.ExecutorVersion.DoubleHashEntriesEnabled() {
		if b, err := body.MarshalBinary(); err != nil {
			return nil, errors.BadRequest.WithFormat("pad synthetic transaction (1): %w", err)
		} else if len(b) == 64 {
			// Add a zero
			b = append(b, 0)

			body, err = protocol.UnmarshalTransactionBody(b)
			if err != nil {
				return nil, errors.BadRequest.WithFormat("pad synthetic transaction (2): %w", err)
			}
		}
	}

	var ledger *protocol.SyntheticLedger
	record := batch.Account(m.Describe.Synthetic())
	err := record.Main().GetAs(&ledger)
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
	txn.Body = body

	initSig, err := new(signing.Builder).
		SetUrl(m.Describe.NodeUrl()).
		SetVersion(destLedger.Produced).
		InitiateSynthetic(txn, destPartUrl)
	if err != nil {
		return nil, err
	}
	m.logger.Debug("Built synthetic transaction", "module", "synthetic", "txid", logging.AsHex(txn.GetHash()), "destination", dest.String(), "type", body.Type(), "seq-num", destLedger.Produced)

	// Update the ledger
	err = record.Main().Put(ledger)
	if err != nil {
		return nil, err
	}

	// Store the transaction, its status, and the initiator
	err = putSyntheticTransaction(
		batch, txn,
		&protocol.TransactionStatus{
			Code:               errors.Remote,
			SourceNetwork:      initSig.SourceNetwork,
			DestinationNetwork: initSig.DestinationNetwork,
			SequenceNumber:     initSig.SequenceNumber,
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

	partition, ok := protocol.ParsePartitionUrl(initSig.DestinationNetwork)
	if !ok {
		return nil, errors.InternalError.WithFormat("destination URL is not a valid partition")
	}

	indexIndex, err := addIndexChainEntry(record.SyntheticSequenceChain(partition), &protocol.IndexEntry{
		Source: uint64(index),
	})
	if err != nil {
		return nil, err
	}
	if indexIndex+1 != initSig.SequenceNumber {
		m.logger.Error("Sequence number does not match index chain index", "seq-num", initSig.SequenceNumber, "index", indexIndex, "source", initSig.SourceNetwork, "destination", initSig.DestinationNetwork)
	}

	return txn, nil
}

func (x *Executor) buildSynthReceipt(batch *database.Batch, produced []*protocol.Transaction, rootAnchor, synthAnchor int64) error {
	// Load the root chain
	chain, err := batch.Account(x.Describe.Ledger()).RootChain().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load root chain: %w", err)
	}

	// Prove the synthetic transaction chain anchor
	rootProof, err := chain.Receipt(synthAnchor, rootAnchor)
	if err != nil {
		return errors.UnknownError.WithFormat("prove from %d to %d on the root chain: %w", synthAnchor, rootAnchor, err)
	}

	// Load the synthetic transaction chain
	chain, err = batch.Account(x.Describe.Synthetic()).MainChain().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load root chain: %w", err)
	}

	synthStart := chain.Height() - int64(len(produced))
	synthEnd := chain.Height() - 1

	// For each produced transaction
	for i, transaction := range produced {
		// TODO Can we make this less hacky?
		record := batch.Transaction(transaction.GetHash())
		status, err := record.Status().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load synthetic transaction status: %w", err)
		}
		if status.SourceNetwork == nil || status.DestinationNetwork == nil || status.SequenceNumber == 0 {
			return errors.InternalError.WithFormat("synthetic transaction %X status is not correctly initialized", transaction.GetHash()[:4])
		}

		// Prove it
		synthProof, err := chain.Receipt(int64(i+int(synthStart)), synthEnd)
		if err != nil {
			return errors.UnknownError.WithFormat("prove from %d to %d on the synthetic transaction chain: %w", i+int(synthStart), synthEnd, err)
		}

		r, err := synthProof.Combine(rootProof)
		if err != nil {
			return errors.UnknownError.WithFormat("combine receipts: %w", err)
		}

		status.Proof = r
		err = record.Status().Put(status)
		if err != nil {
			return errors.UnknownError.WithFormat("store synthetic transaction status: %w", err)
		}
	}

	return nil
}

func processSyntheticTransaction(batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) error {
	// Check that the partition signature has been received
	if status.SourceNetwork == nil {
		return errors.Unauthenticated.WithFormat("missing partition signature")
	}

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

	// Check that the receipt signature has been received
	if status.Proof == nil {
		return errors.Unauthenticated.WithFormat("missing synthetic transaction receipt")
	}

	return nil
}

func putSyntheticTransaction(batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) error {
	// Store the transaction
	obj := batch.Transaction(transaction.GetHash())
	err := obj.Main().Put(&database.SigOrTxn{Transaction: transaction})
	if err != nil {
		return fmt.Errorf("store transaction: %w", err)
	}

	// Update the status
	err = obj.Status().Put(status)
	if err != nil {
		return fmt.Errorf("store status: %w", err)
	}

	return nil
}
