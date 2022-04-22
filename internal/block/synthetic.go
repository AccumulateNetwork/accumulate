package block

import (
	"crypto/sha256"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

func (x *Executor) ProduceSynthetic(batch *database.Batch, from *protocol.Transaction, produced []*protocol.Transaction) error {
	if len(produced) == 0 {
		return nil
	}

	state := new(chain.ChainUpdates)

	for _, sub := range produced {
		tx, err := x.buildSynthTxn(state, batch, sub.Header.Principal, sub.Body)
		if err != nil {
			return err
		}
		sub.Header = tx.Header

		// Don't record txn -> produced synth txn for internal transactions
		if from.Body.Type().IsInternal() {
			continue
		}

		swo, ok := sub.Body.(protocol.SynthTxnWithOrigin)
		if !ok {
			continue
		}

		cause, _ := swo.GetSyntheticOrigin()
		if receipt, ok := sub.Body.(*protocol.SyntheticReceipt); ok {
			cause = receipt.SynthTxHash[:]

			// If the transaction was successful, skip recording the receipt
			if receipt.Status.Code == 0 {
				continue
			}
		}

		err = batch.Transaction(cause).AddSyntheticTxns(*(*[32]byte)(tx.GetHash()))
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *Executor) buildSynthTxn(state *chain.ChainUpdates, batch *database.Batch, dest *url.URL, body protocol.TransactionBody) (*protocol.Transaction, error) {
	// m.logDebug("Built synth txn", "txid", logging.AsHex(tx.GetTxHash()), "dest", dest.String(), "nonce", tx.SigInfo.Nonce, "type", body.Type())

	// Generate a synthetic tx and send to the router. Need to track txid to
	// make sure they get processed.

	var ledgerState *protocol.InternalLedger
	err := batch.Account(m.Network.Ledger()).GetStateAs(&ledgerState)
	if err != nil {
		// If we can't load the ledger, the node is fubared
		panic(fmt.Errorf("failed to load the ledger: %v", err))
	}

	txn := new(protocol.Transaction)
	txn.Header.Principal = dest
	txn.Body = body
	initSig, err := new(signing.Builder).
		SetUrl(m.Network.NodeUrl()).
		SetVersion(ledgerState.Synthetic.Nonce).
		InitiateSynthetic(txn, m.Router)
	if err != nil {
		return nil, err
	}

	// Append the ID
	ledgerState.Synthetic.Unsigned = append(ledgerState.Synthetic.Unsigned, *(*[32]byte)(txn.GetHash()))

	// Increment the nonce
	ledgerState.Synthetic.Nonce++
	err = batch.Account(m.Network.Ledger()).PutState(ledgerState)
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
	err = state.AddChainEntry(batch, m.Network.Ledger(), protocol.SyntheticChain, protocol.ChainTypeTransaction, txn.GetHash(), 0, 0)
	if err != nil {
		return nil, err
	}

	return txn, nil
}

func validateSyntheticEnvelope(net *config.Network, batch *database.Batch, envelope *chain.Delivery) error {
	// TODO Get rid of this hack and actually check the nonce. But first we have
	// to implement transaction batching.
	v := batch.Account(net.NodeUrl()).Index("SeenSynth", envelope.Transaction.GetHash())
	_, err := v.Get()
	switch {
	case err == nil:
		return protocol.Errorf(protocol.ErrorCodeBadNonce, "duplicate synthetic transaction %X", envelope.Transaction.GetHash())
	case errors.Is(err, storage.ErrNotFound):
		// Ok
	default:
		return err
	}

	return validateSyntheticTransactionSignatures(envelope.Transaction, envelope.Signatures)
}

func processSyntheticTransaction(net *config.Network, batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) error {
	// TODO Get rid of this hack and actually check the nonce. But first we have
	// to implement transaction batching.
	v := batch.Account(net.NodeUrl()).Index("SeenSynth", transaction.GetHash())
	_, err := v.Get()
	switch {
	case err == nil:
		return protocol.Errorf(protocol.ErrorCodeBadNonce, "duplicate synthetic transaction %X", transaction.GetHash())
	case errors.Is(err, storage.ErrNotFound):
		err = v.Put([]byte{1})
		if err != nil {
			return err
		}
	default:
		return err
	}

	// Load all of the signatures
	signatures, err := getAllSignatures(batch, batch.Transaction(transaction.GetHash()), status, transaction.Header.Initiator[:])
	if err != nil {
		return err
	}

	return validateSyntheticTransactionSignatures(transaction, signatures)
}

func putSyntheticTransaction(batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus, signature protocol.Signature) error {
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

	// Record the signature against the transaction
	_, err = obj.AddSignature(signature)
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

func assembleSynthReceipt(batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (*protocol.Receipt, *url.URL, error) {
	// Collect receipts
	receipts := map[[32]byte]*protocol.ReceiptSignature{}
	for _, signer := range status.Signers {
		// Load the signature set
		sigset, err := batch.Transaction(transaction.GetHash()).ReadSignaturesForSigner(signer)
		if err != nil {
			return nil, nil, errors.Format(errors.StatusUnknown, "load signatures set %v: %w", signer.GetUrl(), err)
		}

		for _, entryHash := range sigset.EntryHashes() {
			state, err := batch.Transaction(entryHash[:]).GetState()
			if err != nil {
				return nil, nil, errors.Format(errors.StatusUnknown, "load signature entry %X: %w", entryHash, err)
			}

			if state.Signature == nil {
				// This should not happen
				continue
			}

			receipt, ok := state.Signature.(*protocol.ReceiptSignature)
			if !ok {
				continue
			}
			receipts[*(*[32]byte)(receipt.Start)] = receipt
		}
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
