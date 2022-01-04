package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

// addSynthTxns prepares synthetic transactions for signing next block.
func (m *Executor) addSynthTxns(tx *transactions.Envelope, submissions []*SubmittedTransaction) error {
	txid := types.Bytes(tx.Transaction.Hash()).AsBytes32()

	ledger := m.blockBatch.Record(m.Network.NodeUrl().JoinPath(protocol.Ledger))
	chain, err := ledger.Chain(protocol.SyntheticChain)
	if err != nil {
		return err
	}

	// Need to pass this to a threaded batcher / dispatcher to do both signing
	// and sending of synth tx. No need to spend valuable time here doing that.
	ids := make([][32]byte, len(submissions))
	for i, sub := range submissions {
		// Generate a synthetic tx and send to the router. Need to track txid to
		// make sure they get processed.

		tx, err := m.buildSynthTxn(sub.Url, sub.Body, m.blockBatch)
		if err != nil {
			return err
		}

		txPending := state.NewPendingTransaction(tx)
		txState, txPending := state.NewTransaction(txPending)

		status := &protocol.TransactionStatus{Remote: true}
		err = m.blockBatch.Transaction(tx.Transaction.Hash()).Put(txState, status, nil)
		if err != nil {
			return err
		}

		err = chain.AddEntry(tx.Transaction.Hash())
		if err != nil {
			return err
		}

		copy(ids[i][:], tx.Transaction.Hash())
	}

	return m.blockBatch.Transaction(txid[:]).AddSyntheticTxns(ids...)
}

func (opts *ExecutorOptions) buildSynthTxn(dest *url.URL, body protocol.TransactionPayload, batch *database.Batch) (*transactions.Envelope, error) {
	// Marshal the payload
	data, err := body.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal synthetic transaction payload: %v", err)
	}

	// Build the transaction
	env := new(transactions.Envelope)
	env.Transaction = new(transactions.Transaction)
	env.Transaction.Origin = dest
	env.Transaction.KeyPageHeight = 1
	env.Transaction.KeyPageIndex = 0
	env.Transaction.Body = data

	// m.logDebug("Built synth txn", "txid", logging.AsHex(tx.Transaction.Hash()), "dest", dest.String(), "nonce", tx.SigInfo.Nonce, "type", body.GetType())

	ledger := batch.Record(opts.Network.NodeUrl().JoinPath(protocol.Ledger))
	ledgerState := new(protocol.InternalLedger)
	err = ledger.GetStateAs(ledgerState)
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
	txid := types.Bytes(env.Transaction.Hash()).AsBytes32()
	ledgerState.Synthetic.Unsigned = append(ledgerState.Synthetic.Unsigned, txid)

	err = ledger.PutState(ledgerState)
	if err != nil {
		return nil, err
	}

	return env, nil
}
