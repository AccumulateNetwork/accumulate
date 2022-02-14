package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
	"gitlab.com/accumulatenetwork/accumulate/types/state"
)

// addSynthTxns prepares synthetic transactions for signing next block.
func (m *Executor) addSynthTxns(st *stateCache, submissions []*submission) error {
	// Need to pass this to a threaded batcher / dispatcher to do both signing
	// and sending of synth tx. No need to spend valuable time here doing that.
	ids := make([][32]byte, len(submissions))
	for i, sub := range submissions {
		// Generate a synthetic tx and send to the router. Need to track txid to
		// make sure they get processed.

		tx, err := m.buildSynthTxn(st, sub.Url, sub.Body)
		if err != nil {
			return err
		}

		txPending := state.NewPendingTransaction(tx)
		txState, txPending := state.NewTransaction(txPending)

		status := &protocol.TransactionStatus{Remote: true}
		err = m.blockBatch.Transaction(tx.GetTxHash()).Put(txState, status, nil)
		if err != nil {
			return err
		}

		err = st.AddChainEntry(m.Network.NodeUrl(protocol.Ledger), protocol.SyntheticChain, protocol.ChainTypeTransaction, tx.GetTxHash(), 0, 0)
		if err != nil {
			return err
		}

		copy(ids[i][:], tx.GetTxHash())
	}

	ledgerState := protocol.NewInternalLedger()
	err := st.LoadUrlAs(m.Network.NodeUrl(protocol.Ledger), ledgerState)
	if err != nil {
		return err
	}

	ledgerState.Synthetic.Produced = append(ledgerState.Synthetic.Produced, ids...)
	st.Update(ledgerState)
	st.AddSyntheticTxns(st.txHash[:], ids)
	return nil
}

func (opts *ExecutorOptions) buildSynthTxn(st *stateCache, dest *url.URL, body protocol.TransactionPayload) (*transactions.Envelope, error) {
	// Build the transaction
	env := new(transactions.Envelope)
	env.Transaction = new(transactions.Transaction)
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
	txid := types.Bytes(env.GetTxHash()).AsBytes32()
	ledgerState.Synthetic.Unsigned = append(ledgerState.Synthetic.Unsigned, txid)

	st.Update(ledgerState)
	return env, nil
}
