package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

// addSynthTxns prepares synthetic transactions for signing next block.
func (m *Executor) addSynthTxns(st *stateCache, produced []*protocol.Transaction) error {
	ids := make([][32]byte, len(produced))
	for i, sub := range produced {
		// Generate a synthetic tx and send to the router. Need to track txid to
		// make sure they get processed.

		tx, err := m.buildSynthTxn(st, sub.Origin, sub.Body)
		if err != nil {
			return err
		}
		*sub = *tx.Transaction

		status := &protocol.TransactionStatus{Remote: true}
		err = m.blockBatch.Transaction(tx.GetTxHash()).Put(tx.Transaction, status, nil)
		if err != nil {
			return err
		}

		err = st.AddChainEntry(m.Network.NodeUrl(protocol.Ledger), protocol.SyntheticChain, protocol.ChainTypeTransaction, tx.GetTxHash(), 0, 0)
		if err != nil {
			return err
		}

		copy(ids[i][:], tx.GetTxHash())
	}

	st.AddSyntheticTxns(st.txHash[:], ids)
	return nil
}

func (opts *ExecutorOptions) buildSynthTxn(st *stateCache, dest *url.URL, body protocol.TransactionBody) (*protocol.Envelope, error) {
	// Build the transaction
	env := new(protocol.Envelope)
	env.Transaction = new(protocol.Transaction)
	env.Transaction.Origin = dest
	env.Transaction.KeyPageHeight = 1
	env.Transaction.KeyPageIndex = 0
	env.Transaction.Body = body

	// m.logDebug("Built synth txn", "txid", logging.AsHex(tx.GetTxHash()), "dest", dest.String(), "nonce", tx.SigInfo.Nonce, "type", body.GetType())

	ledgerState := new(protocol.InternalLedger)
	err := st.LoadUrlAs(opts.Network.NodeUrl(protocol.Ledger), ledgerState)
	if err != nil {
		// If we can't load the ledger, the node is fubared
		panic(fmt.Errorf("failed to load the ledger: %v", err))
	}

	if body.GetType().IsInternal() {
		// For internal transactions, set the nonce to the height of the next block
		env.Transaction.Nonce = uint64(ledgerState.Index) + 1
		return env, nil
	}

	env.Transaction.Nonce = ledgerState.Synthetic.Nonce

	// Increment the nonce
	ledgerState.Synthetic.Nonce++

	// Append the ID
	if body.Type() == protocol.TransactionTypeSyntheticAnchor {
		txid := types.Bytes(env.GetTxHash()).AsBytes32()
		ledgerState.Synthetic.Unsigned = append(ledgerState.Synthetic.Unsigned, txid)
	}

	st.Update(ledgerState)
	return env, nil
}
