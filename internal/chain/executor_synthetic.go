package chain

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

func (x *Executor) ProduceSynthetic(batch *database.Batch, from *protocol.Transaction, produced []*protocol.Transaction) error {
	if len(produced) == 0 {
		return nil
	}

	st := newStateCache(x.Network.NodeUrl(), from.Body.Type(), *(*[32]byte)(from.GetHash()), batch)
	err := x.addSynthTxns(st, produced)
	if err != nil {
		return err
	}

	_, err = st.Commit()
	if err != nil {
		return err
	}

	return nil
}

// addSynthTxns prepares synthetic transactions for signing next block.
func (m *Executor) addSynthTxns(st *stateCache, produced []*protocol.Transaction) error {
	for _, sub := range produced {
		tx, err := m.buildSynthTxn(st, sub.Header.Principal, sub.Body)
		if err != nil {
			return err
		}
		sub.Header = tx.Header

		// Don't record txn -> produced synth txn for internal transactions
		if st.txType.IsInternal() {
			continue
		}

		// TODO This is a workaround for AC-1238. Once the DN anchor race is
		// fixed, remove this.
		if sub.Body.Type() == protocol.TransactionTypeSyntheticReceipt {
			continue
		}

		swo, ok := sub.Body.(protocol.SynthTxnWithOrigin)
		if !ok {
			// This should not happen. Other than InternalSendTransactions, a
			// transaction should never produce a synthetic transaction that
			// does not have an origin.
			return fmt.Errorf("transaction type %v does not have a synthetic origin", sub.Body.Type())
		}

		cause, _ := swo.GetSyntheticOrigin()
		st.AddSyntheticTxn(cause, *(*[32]byte)(tx.GetHash()))
	}

	return nil
}

func (m *Executor) buildSynthTxn(st *stateCache, dest *url.URL, body protocol.TransactionBody) (*protocol.Transaction, error) {
	// m.logDebug("Built synth txn", "txid", logging.AsHex(tx.GetTxHash()), "dest", dest.String(), "nonce", tx.SigInfo.Nonce, "type", body.GetType())

	// Generate a synthetic tx and send to the router. Need to track txid to
	// make sure they get processed.

	var ledgerState *protocol.InternalLedger
	err := st.LoadUrlAs(m.Network.NodeUrl(protocol.Ledger), &ledgerState)
	if err != nil {
		// If we can't load the ledger, the node is fubared
		panic(fmt.Errorf("failed to load the ledger: %v", err))
	}

	txn := new(protocol.Transaction)
	txn.Header.Principal = dest
	txn.Body = body
	initSig, err := new(signing.Signer).
		SetUrl(m.Network.NodeUrl()).
		SetVersion(ledgerState.Synthetic.Nonce).
		InitiateSynthetic(txn, m.Router)
	if err != nil {
		return nil, err
	}

	// Append the ID
	if body.Type() == protocol.TransactionTypeSyntheticAnchor {
		ledgerState.Synthetic.Unsigned = append(ledgerState.Synthetic.Unsigned, *(*[32]byte)(txn.GetHash()))
	}

	// Increment the nonce
	ledgerState.Synthetic.Nonce++
	st.Update(ledgerState)

	// Store the transaction, its status, and the initiator
	err = putSyntheticTransaction(
		st.batch, txn,
		&protocol.TransactionStatus{Remote: true},
		initSig)
	if err != nil {
		return nil, err
	}

	// Add the transaction to the synthetic transaction chain
	err = st.AddChainEntry(m.Network.NodeUrl(protocol.Ledger), protocol.SyntheticChain, protocol.ChainTypeTransaction, txn.GetHash(), 0, 0)
	if err != nil {
		return nil, err
	}

	return txn, nil
}

func validateSyntheticEnvelope(net *config.Network, batch *database.Batch, envelope *protocol.Envelope) error {
	// TODO Get rid of this hack and actually check the nonce. But first we have
	// to implement transaction batching.
	v := batch.Account(net.NodeUrl()).Index("SeenSynth", envelope.GetTxHash())
	_, err := v.Get()
	switch {
	case err == nil:
		return protocol.Errorf(protocol.ErrorCodeBadNonce, "duplicate synthetic transaction %X", envelope.GetTxHash())
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
	err := obj.PutState(&protocol.Envelope{Transaction: transaction})
	if err != nil {
		return fmt.Errorf("store transaction: %w", err)
	}

	// Update the status
	status.AddSigner(signature.GetSigner())
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
	err = batch.Transaction(sigHash[:]).PutState(&protocol.Envelope{
		TxHash:     transaction.GetHash(),
		Signatures: []protocol.Signature{signature},
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
