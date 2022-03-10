package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// addSynthTxns prepares synthetic transactions for signing next block.
func (m *Executor) addSynthTxns(st *stateCache, produced []*protocol.Transaction) error {
	ids := make([][32]byte, len(produced))
	for i, sub := range produced {
		// Generate a synthetic tx and send to the router. Need to track txid to
		// make sure they get processed.

		tx, err := m.buildSynthTxn(st, sub.Header.Principal, sub.Body)
		if err != nil {
			return err
		}
		sub.Header = tx.Header

		status := &protocol.TransactionStatus{Remote: true}
		err = m.blockBatch.Transaction(tx.GetHash()).Put(tx, status, nil)
		if err != nil {
			return err
		}

		err = st.AddChainEntry(m.Network.NodeUrl(protocol.Ledger), protocol.SyntheticChain, protocol.ChainTypeTransaction, tx.GetHash(), 0, 0)
		if err != nil {
			return err
		}

		copy(ids[i][:], tx.GetHash())
	}

	st.AddSyntheticTxns(st.txHash[:], ids)
	return nil
}

func (opts *ExecutorOptions) buildSynthTxn(st *stateCache, dest *url.URL, body protocol.TransactionBody) (*protocol.Transaction, error) {
	// m.logDebug("Built synth txn", "txid", logging.AsHex(tx.GetTxHash()), "dest", dest.String(), "nonce", tx.SigInfo.Nonce, "type", body.GetType())

	destSubnet, err := opts.Router.Route(dest)
	if err != nil {
		return nil, fmt.Errorf("routing %v: %v", dest, err)
	}

	sig := new(protocol.SyntheticSignature)
	sig.SourceNetwork = opts.Network.NodeUrl()
	sig.DestinationNetwork = protocol.SubnetUrl(destSubnet)

	ledgerState := new(protocol.InternalLedger)
	err = st.LoadUrlAs(opts.Network.NodeUrl(protocol.Ledger), ledgerState)
	if err != nil {
		// If we can't load the ledger, the node is fubared
		panic(fmt.Errorf("failed to load the ledger: %v", err))
	}

	if body.Type().IsInternal() {
		// For internal transactions, set the nonce to the height of the next block
		sig.SequenceNumber = uint64(ledgerState.Index) + 1
	} else {
		sig.SequenceNumber = ledgerState.Synthetic.Nonce

		// Increment the nonce
		ledgerState.Synthetic.Nonce++
	}

	initHash, err := sig.InitiatorHash()
	if err != nil {
		// This should never happen
		panic(fmt.Errorf("failed to calculate the synthetic signature initiator hash: %v", err))
	}

	txn := new(protocol.Transaction)
	txn.Header.Principal = dest
	txn.Header.Initiator = *(*[32]byte)(initHash)
	txn.Body = body

	// Append the ID
	if body.Type() == protocol.TransactionTypeSyntheticAnchor {
		ledgerState.Synthetic.Unsigned = append(ledgerState.Synthetic.Unsigned, *(*[32]byte)(txn.GetHash()))
	}

	if !body.Type().IsInternal() {
		// Internal transactions must not make any writes
		st.Update(ledgerState)
	}

	return txn, nil
}
