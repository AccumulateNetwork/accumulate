package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

// addSynthTxns prepares synthetic transactions for signing next block.
func (m *Executor) addSynthTxns(tx *transactions.GenTransaction, submissions []*SubmittedTransaction) error {
	txid := types.Bytes(tx.TransactionHash()).AsBytes32()

	synth := m.blockBatch.Record(m.Network.NodeUrl().JoinPath(protocol.Synthetic))
	chain, err := synth.Chain(protocol.MainChain)
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
		err = m.blockBatch.Transaction(tx.TransactionHash()).Put(txState, status, nil)
		if err != nil {
			return err
		}

		err = chain.AddEntry(tx.TransactionHash())
		if err != nil {
			return err
		}

		copy(ids[i][:], tx.TransactionHash())
	}

	return m.blockBatch.Transaction(txid[:]).AddSyntheticTxns(ids...)
}

func (m *Executor) addSystemTxns(txns ...*transactions.GenTransaction) error {
	if len(txns) == 0 {
		return nil
	}

	txids := make([][32]byte, len(txns))
	for i, tx := range txns {
		pending := state.NewPendingTransaction(tx)
		state, pending := state.NewTransaction(pending)
		status := new(protocol.TransactionStatus)
		err := m.blockBatch.Transaction(tx.TxHash).Put(state, status, tx.Signature)
		if err != nil {
			return err
		}

		copy(txids[i][:], tx.TransactionHash())
	}

	ledger := m.blockBatch.Record(m.Network.NodeUrl().JoinPath(protocol.Ledger))
	ledgerState := protocol.NewInternalLedger()
	err := ledger.GetStateAs(ledgerState)
	if err != nil {
		return err
	}

	ledgerState.SystemTxns = append(ledgerState.SystemTxns, txids...)
	err = ledger.PutState(ledgerState)
	if err != nil {
		return err
	}

	return nil
}

func (m *Executor) addAnchorTxn() error {
	ledger := m.blockBatch.Record(m.Network.NodeUrl().JoinPath(protocol.Ledger))
	ledgerState := protocol.NewInternalLedger()
	err := ledger.GetStateAs(ledgerState)
	if err != nil {
		return err
	}

	rootChain, err := ledger.Chain(protocol.MinorRootChain)
	if err != nil {
		return err
	}

	synthChain, err := ledger.Chain(protocol.MajorRootChain)
	if err != nil {
		return err
	}

	if m.blockMeta.Deliver.Empty() {
		// Don't create an anchor transaction since no records were updated and
		// no synthetic transactions were produced
		return nil
	}

	m.blockBatch.UpdateBpt()

	body := new(protocol.SyntheticAnchor)
	body.Source = m.Network.NodeUrl().String()
	body.Index = m.blockIndex
	body.Timestamp = m.blockTime
	copy(body.Root[:], m.blockBatch.RootHash())
	body.Chains = ledgerState.Chains
	copy(body.ChainAnchor[:], rootChain.Anchor())
	copy(body.SynthTxnAnchor[:], synthChain.Anchor())

	m.logDebug("Creating anchor txn", "root", logging.AsHex(body.Root), "chains", logging.AsHex(body.ChainAnchor), "synth", logging.AsHex(body.SynthTxnAnchor))

	for _, id := range body.Chains {
		m.logDebug("Anchor includes", "id", logging.AsHex(id))
	}

	var txns []*transactions.GenTransaction
	switch m.Network.Type {
	case config.Directory:
		// Send anchors from DN to all BVNs
		for _, bvn := range m.Network.BvnNames {
			tx, err := m.buildSynthTxn(protocol.BvnUrl(bvn), body, m.blockBatch)
			if err != nil {
				return err
			}
			txns = append(txns, tx)
		}

	case config.BlockValidator:
		// Send anchor from BVN to DN
		tx, err := m.buildSynthTxn(protocol.DnUrl(), body, m.blockBatch)
		if err != nil {
			return err
		}
		txns = append(txns, tx)
	}

	return m.addSystemTxns(txns...)
}

func (opts *ExecutorOptions) buildSynthTxn(dest *url.URL, body protocol.TransactionPayload, batch *database.Batch) (*transactions.GenTransaction, error) {
	// Marshal the payload
	data, err := body.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal synthetic transaction payload: %v", err)
	}

	// Build the transaction
	tx := new(transactions.GenTransaction)
	tx.SigInfo = new(transactions.SignatureInfo)
	tx.SigInfo.URL = dest.String()
	tx.SigInfo.KeyPageHeight = 1
	tx.SigInfo.KeyPageIndex = 0
	tx.Transaction = data

	// m.logDebug("Built synth txn", "txid", logging.AsHex(tx.TransactionHash()), "dest", dest.String(), "nonce", tx.SigInfo.Nonce, "type", body.GetType())

	if body.GetType().IsInternal() {
		tx.SigInfo.Nonce = 1
		return tx, nil
	}

	ledger := batch.Record(opts.Network.NodeUrl().JoinPath(protocol.Ledger))
	ledgerState := new(protocol.InternalLedger)
	err = ledger.GetStateAs(ledgerState)
	if err != nil {
		// If we can't load the ledger, the node is fubared
		panic(fmt.Errorf("failed to load the ledger: %v", err))
	}

	tx.SigInfo.Nonce = ledgerState.Synthetic.Nonce

	// Increment the nonce
	ledgerState.Synthetic.Nonce++

	// Append the ID
	txid := types.Bytes(tx.TransactionHash()).AsBytes32()
	ledgerState.Synthetic.Unsigned = append(ledgerState.Synthetic.Unsigned, txid)

	err = ledger.PutState(ledgerState)
	if err != nil {
		return nil, err
	}

	return tx, nil
}
