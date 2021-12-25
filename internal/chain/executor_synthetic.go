package chain

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/abci"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

// addSynthTxns prepares synthetic transactions for signing next block.
func (m *Executor) addSynthTxns(parentTxId types.Bytes, submissions []*SubmittedTransaction) error {
	synth := m.blockBatch.Record(m.Network.NodeUrl().JoinPath(protocol.Synthetic))
	chain, err := synth.Chain(protocol.Main)
	if err != nil {
		return err
	}

	// Need to pass this to a threaded batcher / dispatcher to do both signing
	// and sending of synth tx. No need to spend valuable time here doing that.
	ids := make([][32]byte, len(submissions))
	for i, sub := range submissions {
		// Generate a synthetic tx and send to the router. Need to track txid to
		// make sure they get processed.

		tx, err := m.buildSynthTxn(sub.url, sub.body)
		if err != nil {
			return err
		}

		txPending := state.NewPendingTransaction(tx)
		txState, txPending := state.NewTransaction(txPending)

		err = m.blockBatch.Transaction(tx.TransactionHash()).PutState(txState)
		if err != nil {
			return err
		}

		err = chain.AddEntry(tx.TransactionHash())
		if err != nil {
			return err
		}

		copy(ids[i][:], tx.TransactionHash())
	}

	return m.blockBatch.Transaction(parentTxId).AddSyntheticTxns(ids...)
}

func (m *Executor) addSystemTxns(txns ...*transactions.GenTransaction) error {
	if len(txns) == 0 {
		return nil
	}

	txids := make([][32]byte, len(txns))
	for i, tx := range txns {
		pending := state.NewPendingTransaction(tx)
		state, pending := state.NewTransaction(pending)
		err := m.blockBatch.Transaction(tx.TxHash).Put(state, nil, tx.Signature)
		if err != nil {
			return err
		}

		copy(txids[i][:], tx.TransactionHash())
	}

	state := new(state.Anchor)
	root := m.blockBatch.Record(m.Network.NodeUrl().JoinPath(protocol.MinorRoot))
	err := root.GetStateAs(state)
	if err != nil {
		return err
	}

	state.SystemTxns = append(state.SystemTxns, txids...)
	err = root.PutState(state)
	if err != nil {
		return err
	}

	return nil
}

func (m *Executor) addAnchorTxn() error {
	root := m.blockBatch.Record(m.Network.NodeUrl().JoinPath(protocol.MinorRoot))
	synth := m.blockBatch.Record(m.Network.NodeUrl().JoinPath(protocol.Synthetic))
	rootHead, synthHead := state.NewAnchor(), state.NewSyntheticTransactionChain()

	err := root.GetStateAs(rootHead)
	if err != nil {
		return err
	}

	err = synth.GetStateAs(synthHead)
	if err != nil {
		return err
	}

	rootChain, err := root.Chain(protocol.Main)
	if err != nil {
		return err
	}

	synthChain, err := synth.Chain(protocol.Main)
	if err != nil {
		return err
	}

	switch {
	case rootHead.Index == m.blockIndex && len(rootHead.Chains) > 0:
		// Modified chains last block, continue
	case synthHead.Index == m.blockIndex:
		// Produced synthetic transactions last block, continue
	default:
		// Nothing happened last block, so skip creating an anchor txn
		return nil
	}

	body := new(protocol.SyntheticAnchor)
	body.Source = m.Network.NodeUrl().String()
	body.Index = m.blockIndex
	body.Timestamp = m.blockTime
	copy(body.Root[:], m.blockBatch.RootHash())
	body.Chains = rootHead.Chains
	copy(body.ChainAnchor[:], rootChain.Anchor())
	copy(body.SynthTxnAnchor[:], synthChain.Anchor())

	m.logDebug("Creating anchor txn", "root", logging.AsHex(body.Root), "chains", logging.AsHex(body.ChainAnchor), "synth", logging.AsHex(body.SynthTxnAnchor))

	var txns []*transactions.GenTransaction
	switch m.Network.Type {
	case config.Directory:
		// Send anchors from DN to all BVNs
		for _, bvn := range m.Network.BvnNames {
			tx, err := m.buildSynthTxn(protocol.BvnUrl(bvn), body)
			if err != nil {
				return err
			}
			txns = append(txns, tx)
		}

	case config.BlockValidator:
		// Send anchor from BVN to DN
		tx, err := m.buildSynthTxn(protocol.DnUrl(), body)
		if err != nil {
			return err
		}
		txns = append(txns, tx)
	}

	return m.addSystemTxns(txns...)
}

func (m *Executor) buildSynthTxn(dest *url.URL, body protocol.TransactionPayload) (*transactions.GenTransaction, error) {
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

	// Load the chain state
	synth := m.blockBatch.Record(m.Network.NodeUrl().JoinPath(protocol.Synthetic))
	head := state.NewSyntheticTransactionChain()
	err = synth.GetStateAs(head)
	if err != nil {
		return nil, err
	}

	// Increment the nonce
	head.Nonce++
	tx.SigInfo.Nonce = uint64(head.Nonce)

	// Save the updated chain state
	err = synth.PutState(head)
	if err != nil {
		return nil, err
	}

	m.logDebug("Built synth txn", "txid", logging.AsHex(tx.TransactionHash()), "dest", dest.String(), "nonce", tx.SigInfo.Nonce, "type", body.GetType())

	return tx, nil
}

// signSynthTxns signs synthetic transactions from the previous block and
// prepares them to be sent next block.
func (m *Executor) signSynthTxns() error {
	// TODO If the leader fails, will the block happen again or will Tendermint
	// move to the next block? We need to be sure that synthetic transactions
	// won't get lost.

	// Retrieve transactions from the previous block
	txns, err := m.synthTxnsLastBlock(m.blockIndex - 1)
	if err != nil {
		return err
	}

	// Check for anchor transactions from the previous block
	root := state.NewAnchor()
	err = m.blockBatch.Record(m.Network.NodeUrl().JoinPath(protocol.MinorRoot)).GetStateAs(root)
	if err != nil {
		return err
	}

	for _, txn := range root.SystemTxns {
		// Copy the variable. Otherwise we end up with a pointer to the loop
		// variable, which means we get multiple copies of the pointer, but
		// they all point to the same thing.
		txn := txn
		txns = append(txns, txn[:])
	}

	// Only proceed if we have transactions to sign
	if len(txns) == 0 {
		return nil
	}

	// Sign all of the transactions
	body := new(protocol.SyntheticSignTransactions)
	for _, txid := range txns {
		// For each pending synthetic transaction
		var synthSig protocol.SyntheticSignature
		copy(synthSig.Txid[:], txid)

		// Load the transaction state
		tx, err := m.blockBatch.Transaction(txid).GetState()
		if err != nil {
			return err
		}

		m.logDebug("Signing synth txn", "txid", logging.AsHex(txid), "type", tx.TxType())

		// Sign it
		ed := new(transactions.ED25519Sig)
		ed.PublicKey = m.Key[32:]
		synthSig.Nonce = tx.SigInfo.Nonce
		err = ed.Sign(synthSig.Nonce, m.Key, txid[:])
		if err != nil {
			return err
		}

		// Add it to the list
		synthSig.Signature = ed.Signature
		body.Transactions = append(body.Transactions, synthSig)
	}

	// Construct the signature transaction
	tx, err := m.buildSynthTxn(m.Network.NodeUrl(), body)
	if err != nil {
		return err
	}

	// Sign it
	ed := new(transactions.ED25519Sig)
	tx.Signature = append(tx.Signature, ed)
	ed.PublicKey = m.Key[32:]
	err = ed.Sign(tx.SigInfo.Nonce, m.Key, tx.TransactionHash())
	if err != nil {
		return err
	}

	// Marshal it
	data, err := tx.Marshal()
	if err != nil {
		return err
	}

	// Only the leader should actually send the transaction
	if !m.blockLeader {
		return nil
	}

	// Send it
	go func() {
		_, err = m.Local.BroadcastTxAsync(context.Background(), data)
		if err != nil {
			m.logError("Failed to broadcast synth txn sigs", "error", err)
		}
	}()
	return nil
}

// sendSynthTxns sends signed synthetic transactions from previous blocks.
//
// Note, only the leader actually sends the transaction, but every other node
// must make the same updates to the database, otherwise consensus will fail,
// since constructing the synthetic transaction updates the nonce, which changes
// the BPT, so everyone needs to do that.
func (m *Executor) sendSynthTxns() ([]abci.SynthTxnReference, error) {
	synth := m.blockBatch.Record(m.Network.NodeUrl().JoinPath(protocol.Synthetic))
	synthState := state.NewSyntheticTransactionChain()
	err := synth.GetStateAs(synthState)
	if err != nil {
		return nil, err
	}

	// Is there anything to send?
	if len(synthState.Signatures) == 0 {
		return nil, nil
	}

	// Array for synth TXN references
	refs := make([]abci.SynthTxnReference, 0, len(synthState.Signatures))

	// Process all the transactions
	sent := map[[32]byte]bool{}
	for _, sig := range synthState.Signatures {
		tx, _, sigs, err := m.blockBatch.Transaction(sig.Txid[:]).Get()
		if err != nil {
			return nil, err
		}

		// Convert it back to a transaction
		gtx := tx.Restore()
		gtx.Signature = sigs

		// Add the signature
		gtx.Signature = append(gtx.Signature, &transactions.ED25519Sig{
			Nonce:     sig.Nonce,
			PublicKey: sig.PublicKey,
			Signature: sig.Signature,
		})

		// Marshal the transaction
		raw, err := gtx.Marshal()
		if err != nil {
			return nil, err
		}

		// Parse the URL
		u, err := url.Parse(gtx.SigInfo.URL)
		if err != nil {
			return nil, err
		}

		// Add it to the batch
		m.logDebug("Sending synth txn", "actor", u.String(), "txid", logging.AsHex(tx.TransactionHash()))
		m.dispatcher.BroadcastTxAsync(context.Background(), u, raw)

		// Delete the signature
		sent[sig.Txid] = true

		// Add the synthetic transaction reference
		var ref abci.SynthTxnReference
		ref.Type = uint64(gtx.TransactionType())
		ref.Url = tx.SigInfo.URL
		ref.TxRef = sha256.Sum256(raw)
		copy(ref.Hash[:], gtx.TransactionHash())
		refs = append(refs, ref)
	}

	sigs := synthState.Signatures
	synthState.Signatures = make([]state.SyntheticSignature, 0, len(sigs)-len(sent))
	for _, sig := range sigs {
		if sent[sig.Txid] {
			continue
		}

		synthState.Signatures = append(synthState.Signatures, sig)
	}

	err = synth.PutState(synthState)
	if err != nil {
		return nil, err
	}

	return refs, nil
}
