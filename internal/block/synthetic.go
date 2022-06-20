package block

import (
	"crypto/sha256"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
)

func (x *Executor) ProduceSynthetic(batch *database.Batch, from *protocol.Transaction, produced []*protocol.Transaction) error {
	if len(produced) == 0 {
		return nil
	}

	err := setSyntheticOrigin(batch, from, produced)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
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

		_, ok := sub.Body.(protocol.SynthTxnWithOrigin)
		if !ok {
			continue
		}

		err = batch.Transaction(from.GetHash()).AddSyntheticTxns(tx.ID())
		if err != nil {
			return err
		}
	}

	return nil
}

// setSyntheticOrigin sets the synthetic origin data of the synthetic
// transaction. setSyntheticOrigin sets the refund amount for each synthetic
// transaction, spreading the potential refund across all produced synthetic
// transactions.
func setSyntheticOrigin(batch *database.Batch, from *protocol.Transaction, produced []*protocol.Transaction) error {
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

	// Get the fee
	paid, err := protocol.ComputeTransactionFee(from)
	if err != nil {
		return errors.Format(errors.StatusInternalError, "compute fee: %w", err)
	}
	if paid <= protocol.FeeFailedMaximum {
		return nil
	}

	status, err := batch.Transaction(from.GetHash()).GetStatus()
	if err != nil {
		return errors.Format(errors.StatusUnknown, "load status: %w", err)
	}

	// Set the refund amount
	refund := (paid - protocol.FeeFailedMaximum) / protocol.Fee(len(swos))
	for _, swo := range swos {
		swo.SetRefund(status.Initiator, refund)
	}
	return nil
}

func (m *Executor) buildSynthTxn(state *chain.ChainUpdates, batch *database.Batch, dest *url.URL, body protocol.TransactionBody) (*protocol.Transaction, error) {
	// m.logDebug("Built synth txn", "txid", logging.AsHex(tx.GetTxHash()), "dest", dest.String(), "nonce", tx.SigInfo.Nonce, "type", body.Type())

	// Generate a synthetic tx and send to the router. Need to track txid to
	// make sure they get processed.

	var ledger *protocol.SyntheticLedger
	record := batch.Account(m.Describe.Synthetic())
	err := record.GetStateAs(&ledger)
	if err != nil {
		// If we can't load the ledger, the node is fubared
		panic(fmt.Errorf("failed to load the ledger: %v", err))
	}

	txn := new(protocol.Transaction)
	txn.Header.Principal = dest
	txn.Body = body
	initSig, err := new(signing.Builder).
		SetUrl(m.Describe.NodeUrl()).
		InitiateSynthetic(txn, m.Router, ledger)
	if err != nil {
		return nil, err
	}

	// Update the ledger
	err = record.PutState(ledger)
	if err != nil {
		return nil, err
	}

	// Store the transaction, its status, and the initiator
	err = m.putSyntheticTransaction(
		batch, txn,
		&protocol.TransactionStatus{Remote: true},
		initSig)
	if err != nil {
		return nil, err
	}

	// Add the transaction to the synthetic transaction chain
	chain, err := record.Chain(protocol.MainChain, protocol.ChainTypeTransaction)
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

	subnet, ok := protocol.ParseSubnetUrl(initSig.DestinationNetwork)
	if !ok {
		return nil, errors.Format(errors.StatusInternalError, "destination URL is not a valid subnet")
	}

	indexIndex, err := addIndexChainEntry(record, protocol.SyntheticIndexChain(subnet), &protocol.IndexEntry{
		Source: uint64(index),
	})
	if err != nil {
		return nil, err
	}
	if indexIndex+1 != uint64(initSig.SequenceNumber) {
		m.logger.Error("Sequence number does not match index chain index", "seq-num", initSig.SequenceNumber, "index", indexIndex, "source", initSig.SourceNetwork, "destination", initSig.DestinationNetwork)
	}

	return txn, nil
}

func (x *Executor) buildSynthReceipt(batch *database.Batch, produced []*protocol.Transaction, rootAnchor, synthAnchor int64) error {
	// Load the root chain
	chain, err := batch.Account(x.Describe.Ledger()).ReadChain(protocol.MinorRootChain)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "load root chain: %w", err)
	}

	// Prove the synthetic transaction chain anchor
	rootProof, err := chain.Receipt(int64(synthAnchor), int64(rootAnchor))
	if err != nil {
		return errors.Format(errors.StatusUnknown, "prove from %d to %d on the root chain: %w", synthAnchor, rootAnchor, err)
	}

	// Load the synthetic transaction chain
	chain, err = batch.Account(x.Describe.Synthetic()).ReadChain(protocol.MainChain)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "load root chain: %w", err)
	}

	synthStart := chain.Height() - int64(len(produced))
	synthEnd := chain.Height() - 1

	// For each produced transaction
	for i, transaction := range produced {
		// TODO Can we make this less hacky?
		record := batch.Transaction(transaction.GetHash())
		status, err := record.GetStatus()
		if err != nil {
			return errors.Format(errors.StatusUnknown, "load synthetic transaction status: %w", err)
		}
		sigs, err := GetAllSignatures(batch, record, status, transaction.Header.Initiator[:])
		if err != nil {
			return errors.Format(errors.StatusUnknown, "load synthetic transaction signatures: %w", err)
		}
		if len(sigs) == 0 {
			return errors.Format(errors.StatusInternalError, "synthetic transaction %X does not have a synthetic origin signature", transaction.GetHash()[:4])
		}
		if len(sigs) > 1 {
			return errors.Format(errors.StatusInternalError, "synthetic transaction %X has more than one signature", transaction.GetHash()[:4])
		}

		// Prove it
		synthProof, err := chain.Receipt(int64(i+int(synthStart)), int64(synthEnd))
		if err != nil {
			return errors.Format(errors.StatusUnknown, "prove from %d to %d on the synthetic transaction chain: %w", i+int(synthStart), synthEnd, err)
		}

		r, err := synthProof.Combine(rootProof)
		if err != nil {
			return errors.Format(errors.StatusUnknown, "combine receipts: %w", err)
		}

		proofSig := new(protocol.ReceiptSignature)
		proofSig.SourceNetwork = x.Describe.NodeUrl()
		proofSig.TransactionHash = *(*[32]byte)(transaction.GetHash())
		proofSig.Proof = *r

		// Record the proof signature but DO NOT record the key signature! Each
		// node has a different key, so recording the key signature here would
		// cause a consensus failure!
		err = batch.Transaction(proofSig.Hash()).PutState(&database.SigOrTxn{
			Txid:      transaction.ID(),
			Signature: proofSig,
		})
		if err != nil {
			return errors.Format(errors.StatusUnknown, "store signature: %w", err)
		}
		_, err = batch.Transaction(transaction.GetHash()).AddSystemSignature(&x.Describe, proofSig)
		if err != nil {
			return errors.Format(errors.StatusUnknown, "record receipt for %X: %w", transaction.GetHash()[:4], err)
		}
	}

	return nil
}

func processSyntheticTransaction(batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) error {
	// Load all of the signatures
	signatures, err := GetAllSignatures(batch, batch.Transaction(transaction.GetHash()), status, transaction.Header.Initiator[:])
	if err != nil {
		return err
	}

	// Validate signatures
	return validateSyntheticTransactionSignatures(transaction, signatures)
}

func (x *Executor) putSyntheticTransaction(batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus, signature *protocol.SyntheticSignature) error {
	// Store the transaction
	obj := batch.Transaction(transaction.GetHash())
	err := obj.PutState(&database.SigOrTxn{Transaction: transaction})
	if err != nil {
		return fmt.Errorf("store transaction: %w", err)
	}

	// Update the status
	err = obj.PutStatus(status)
	if err != nil {
		return fmt.Errorf("store status: %w", err)
	}

	if signature == nil {
		return nil
	}

	// Record the signature against the transaction
	_, err = obj.AddSystemSignature(&x.Describe, signature)
	if err != nil {
		return fmt.Errorf("add signature: %w", err)
	}

	// Hash the signature
	sigData, err := signature.MarshalBinary()
	if err != nil {
		return fmt.Errorf("marshal signature: %w", err)
	}
	sigHash := sha256.Sum256(sigData)

	// Store the signature
	err = batch.Transaction(sigHash[:]).PutState(&database.SigOrTxn{
		Txid:      transaction.ID(),
		Signature: signature,
	})
	if err != nil {
		return fmt.Errorf("store signature: %w", err)
	}

	// // Add the signature to the principal's chain
	// chain, err := batch.Account(transaction.Header.Principal).Chain(protocol.SignatureChain, protocol.ChainTypeTransaction)
	// if err != nil {
	// 	return fmt.Errorf("load chain: %w", err)
	// }
	// err = chain.AddEntry(sigHash[:], true)
	// if err != nil {
	// 	return fmt.Errorf("store chain: %w", err)
	// }

	return nil
}

func assembleSynthReceipt(hash [32]byte, signatures []protocol.Signature) (*managed.Receipt, *url.URL, error) {
	// Collect receipts
	receipts := map[[32]byte]*managed.Receipt{}
	var sourceNet *url.URL
	for _, signature := range signatures {
		receipt, ok := signature.(*protocol.ReceiptSignature)
		if !ok {
			continue
		}
		sourceNet = receipt.SourceNetwork
		receipts[*(*[32]byte)(receipt.Proof.Start)] = &receipt.Proof
	}

	// Get the first
	receipt, ok := receipts[hash]
	delete(receipts, hash)
	if !ok {
		return nil, nil, errors.Format(errors.StatusInternalError, "missing initial receipt")
	}

	// Join the remaining receipts
	for len(receipts) > 0 {
		// Find the next receipt
		hash = *(*[32]byte)(receipt.Anchor)
		next, ok := receipts[hash]
		if !ok {
			// TODO Reject receipts that do not match up. We should probably
			// just store all of this in the transaction status or something.
			break
			// return nil, nil, errors.Format(errors.StatusInternalError, "extra receipts")
		}
		delete(receipts, hash)

		r, err := receipt.Combine(next)
		if err != nil {
			return nil, nil, errors.Format(errors.StatusInternalError, "combine receipts: %w", err)
		}
		receipt = r
	}

	return receipt, sourceNet, nil
}
