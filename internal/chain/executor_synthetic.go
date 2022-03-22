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

	destSubnet, err := m.Router.RouteAccount(dest)
	if err != nil {
		return nil, fmt.Errorf("routing %v: %v", dest, err)
	}

	var ledgerState *protocol.InternalLedger
	err = st.LoadUrlAs(m.Network.NodeUrl(protocol.Ledger), &ledgerState)
	if err != nil {
		// If we can't load the ledger, the node is fubared
		panic(fmt.Errorf("failed to load the ledger: %v", err))
	}

	initSig := new(protocol.SyntheticSignature)
	initSig.SourceNetwork = m.Network.NodeUrl()
	initSig.DestinationNetwork = protocol.SubnetUrl(destSubnet)
	initSig.SequenceNumber = ledgerState.Synthetic.Nonce

	initHash, err := initSig.InitiatorHash()
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

	// Increment the nonce
	ledgerState.Synthetic.Nonce++
	st.Update(ledgerState)

	// Store the transaction, its status, and the initiator
	status := &protocol.TransactionStatus{Remote: true}
	err = m.blockBatch.Transaction(txn.GetHash()).Put(txn, status, []protocol.Signature{initSig})
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
