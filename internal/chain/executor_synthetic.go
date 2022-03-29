package chain

import (
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

func (x *Executor) ProduceSynthetic(batch *database.Batch, from *protocol.Transaction, produced []*protocol.Transaction) (err error) {

	var signer protocol.SignerAccount
	signerUrl := x.Network.ValidatorPage(0)
	err = x.blockBatch.Account(signerUrl).GetStateAs(&signer)
	if err != nil {
		return err
	}

	st := NewStateManager(x.blockBatch.Begin, x.Network.NodeUrl(), signerUrl, signer, nil, from)
	st.logger.L = x.logger

	srEnv := x.blockState.SynthReceiptEnvelope
	if srEnv != nil {
		st.Submit(srEnv.DestUrl, srEnv.SyntheticReceipt)
		x.blockState.SynthReceiptEnvelope = nil
	}

	if produced != nil {
		err = x.addSynthTxns(&st.stateCache, produced)
		if err != nil {
			return err
		}
	}

	err = st.Commit()
	if err != nil {
		return err
	}

	return nil
}

// addSynthTxns prepares synthetic transactions for signing next block.
func (m *Executor) addSynthTxns(st *stateCache, produced []*protocol.Transaction) error {
	ids := make([][32]byte, len(produced))
	for i, sub := range produced {
		tx, err := m.buildSynthTxn(st, sub.Header.Principal, sub.Body)
		if err != nil {
			return err
		}
		sub.Header = tx.Header
		copy(ids[i][:], tx.GetHash())
	}

	st.AddSyntheticTxns(st.txHash[:], ids)
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
		SetHeight(ledgerState.Synthetic.Nonce).
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
	env := new(protocol.Envelope)
	env.Transaction = txn
	status := &protocol.TransactionStatus{Remote: true}
	err = m.blockBatch.Transaction(txn.GetHash()).Put(env, status, []protocol.Signature{initSig})
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

func processSyntheticTransaction(net *config.Network, batch *database.Batch, transaction *protocol.Transaction, signatures []protocol.Signature) error {
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

	return validateSyntheticTransactionSignatures(transaction, signatures)
}
