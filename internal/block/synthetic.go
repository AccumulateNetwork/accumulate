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

		err = batch.Transaction(from.GetHash()).AddSyntheticTxns(*(*[32]byte)(tx.GetHash()))
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
		swo.SetCause(from.GetHash(), from.Header.Principal)
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
	err := batch.Account(m.Network.Synthetic()).GetStateAs(&ledger)
	if err != nil {
		// If we can't load the ledger, the node is fubared
		panic(fmt.Errorf("failed to load the ledger: %v", err))
	}

	txn := new(protocol.Transaction)
	txn.Header.Principal = dest
	txn.Body = body
	initSig, err := new(signing.Builder).
		SetUrl(m.Network.NodeUrl()).
		InitiateSynthetic(txn, m.Router, ledger)
	if err != nil {
		return nil, err
	}

	// Update the ledger
	err = batch.Account(m.Network.Synthetic()).PutState(ledger)
	if err != nil {
		return nil, err
	}

	// Store the transaction, its status, and the initiator
	err = putSyntheticTransaction(
		batch, txn,
		&protocol.TransactionStatus{Remote: true},
		initSig)
	if err != nil {
		return nil, err
	}

	// Add the transaction to the synthetic transaction chain
	err = state.AddChainEntry(batch, m.Network.Synthetic(), protocol.MainChain, protocol.ChainTypeTransaction, txn.GetHash(), 0, 0)
	if err != nil {
		return nil, err
	}

	return txn, nil
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

func putSyntheticTransaction(batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus, signature *protocol.SyntheticSignature) error {
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
	_, err = obj.AddSignature(0, signature)
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
		Hash:      *(*[32]byte)(transaction.GetHash()),
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

func assembleSynthReceipt(transaction *protocol.Transaction, signatures []protocol.Signature) (*protocol.Receipt, *url.URL, error) {
	// Collect receipts
	receipts := map[[32]byte]*protocol.ReceiptSignature{}
	for _, signature := range signatures {
		receipt, ok := signature.(*protocol.ReceiptSignature)
		if !ok {
			continue
		}
		receipts[*(*[32]byte)(receipt.Start)] = receipt
	}

	// Get the first
	hash := *(*[32]byte)(transaction.GetHash())
	rsig, ok := receipts[hash]
	delete(receipts, hash)
	if !ok {
		return nil, nil, nil
	}
	sourceNet := rsig.SourceNetwork

	// Join the remaining receipts
	receipt := &rsig.Receipt
	for len(receipts) > 0 {
		hash = *(*[32]byte)(rsig.Result)
		rsig, ok := receipts[hash]
		delete(receipts, hash)
		if !ok {
			continue
		}

		r := receipt.Combine(&rsig.Receipt)
		if r != nil {
			receipt = r
			sourceNet = rsig.SourceNetwork
		}
	}

	return receipt, sourceNet, nil
}
