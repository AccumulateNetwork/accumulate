package chain

import (
	"context"
	"crypto/sha256"
	"encoding"
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/abci"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/common"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

// synthCount returns the number of synthetic transactions sent by this subnet.
func (m *Executor) synthCount() (uint64, error) {
	k := storage.ComputeKey("SyntheticTransactionCount")
	b, err := m.dbTx.Read(k)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return 0, err
	}

	var n uint64
	if len(b) > 0 {
		n, _ = common.BytesUint64(b)
	}
	return n, nil
}

// nextSynthCount returns and increments the number of synthetic transactions
// sent by this subnet.
func (m *Executor) nextSynthCount() (uint64, error) {
	// TODO Replace this with the actual key nonce

	n, err := m.synthCount()
	if err != nil {
		return 0, err
	}
	n++

	k := storage.ComputeKey("SyntheticTransactionCount")
	m.dbTx.Write(k, common.Uint64Bytes(n))
	return n, nil
}

// addSynthTxns prepares synthetic transactions for signing next block.
func (m *Executor) addSynthTxns(parentTxId types.Bytes, st *StateManager) error {
	// Need to pass this to a threaded batcher / dispatcher to do both signing
	// and sending of synth tx. No need to spend valuable time here doing that.
	for _, sub := range st.submissions {
		// Generate a synthetic tx and send to the router. Need to track txid to
		// make sure they get processed.

		tx, err := m.buildSynthTxn(parentTxId, sub.url, sub.body)
		if err != nil {
			return err
		}

		txSynthetic := state.NewPendingTransaction(tx)
		txSyntheticObject := new(state.Object)
		synthTxData, err := txSynthetic.MarshalBinary()
		if err != nil {
			return err
		}

		txSyntheticObject.Entry = synthTxData
		m.dbTx.AddSynthTx(parentTxId, tx.TransactionHash(), txSyntheticObject)
	}

	return nil
}

func (m *Executor) addAnchorTxn(height int64) ([][]byte, error) {
	srcUrl, err := nodeUrl(m.DB, m.isDirectory)
	if err != nil {
		return nil, err
	}

	synth, err := m.DB.SynthTxidChain()
	if err != nil {
		return nil, err
	}

	synthHead, err := synth.Record()
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	}

	anchor, err := m.DB.MinorAnchorChain()
	if err != nil {
		return nil, err
	}

	anchorHead, err := anchor.Record()
	if err != nil {
		return nil, err
	}

	switch {
	case anchorHead.Index == height-1 && len(anchorHead.Chains) > 0:
		// Modified chains last block, continue
	case synthHead.Index == height-1:
		// Produced synthetic transactions last block, continue
	default:
		// Nothing happened last block, so skip creating an anchor txn
		return nil, nil
	}

	body := new(protocol.SyntheticAnchor)
	body.Source = srcUrl.String()
	body.Index = m.height
	body.Timestamp = m.time
	copy(body.Root[:], m.DB.RootHash())
	body.Chains = anchorHead.Chains
	copy(body.ChainAnchor[:], anchor.Chain.Anchor())
	copy(body.SynthTxnAnchor[:], synth.Chain.Anchor())

	m.logger.Debug("Creating anchor txn", "root", logging.AsHex(body.Root), "chains", logging.AsHex(body.ChainAnchor), "synth", logging.AsHex(body.SynthTxnAnchor))

	var txns []*transactions.GenTransaction
	switch {
	case m.isDirectory:
		// TODO Send anchors from DN to all BVNs

	case m.Directory != "":
		// Send anchor from BVN to DN
		tx, err := m.buildSynthTxn(make([]byte, 32), dnUrl(), body)
		if err != nil {
			return nil, err
		}
		txns = append(txns, tx)

	default:
		// If the Directory is not specified, do not send the anchor TXN
	}

	var txids [][]byte
	for _, tx := range txns {
		obj := new(state.Object)
		obj.Entry, err = state.NewPendingTransaction(tx).MarshalBinary()
		if err != nil {
			return nil, err
		}

		err = m.dbTx.WriteSynthTxn(tx.TransactionHash(), obj)
		if err != nil {
			return nil, err
		}

		txids = append(txids, tx.TransactionHash())
	}

	return txids, nil
}

func (m *Executor) buildSynthTxn(parentTxid []byte, dest *url.URL, body encoding.BinaryMarshaler) (*transactions.GenTransaction, error) {
	data, err := body.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal synthetic transaction payload: %v", err)
	}

	tx := new(transactions.GenTransaction)
	tx.SigInfo = new(transactions.SignatureInfo)
	tx.SigInfo.URL = dest.String()
	tx.SigInfo.KeyPageHeight = 1
	tx.SigInfo.KeyPageIndex = 0
	tx.Transaction = data

	tx.SigInfo.KeyPageHeight = 1
	tx.SigInfo.Nonce, err = m.nextSynthCount()
	if err != nil {
		return nil, err
	}

	return tx, nil
}

// signSynthTxns signs synthetic transactions from the previous block and
// prepares them to be sent next block.
func (m *Executor) signSynthTxns(txns [][]byte) error {
	// TODO If the leader fails, will the block happen again or will Tendermint
	// move to the next block? We need to be sure that synthetic transactions
	// won't get lost.

	// Load the synth txid chain
	chain, err := m.DB.SynthTxidChain()
	if err != nil {
		return err
	}

	// Retrieve transactions from the previous block
	prevBlockTxns, err := chain.LastBlock(m.height - 1)
	if err != nil {
		return err
	}

	// Transactions from the block were signed first
	txns = append(prevBlockTxns, txns...)

	// Only proceed if we have transactions to sign
	if len(txns) == 0 {
		return nil
	}

	// Use the synthetic transaction count to calculate what the nonces were
	nonce, err := m.synthCount()
	if err != nil {
		return err
	}

	// Sign all of the transactions
	body := new(protocol.SyntheticSignTransactions)
	for i, txid := range txns {
		m.logger.Debug("Signing synth txn", "txid", logging.AsHex(txid))

		// For each pending synthetic transaction
		var synthSig protocol.SyntheticSignature
		copy(synthSig.Txid[:], txid)

		// The nonce must be the final nonce minus (I + 1)
		synthSig.Nonce = nonce - 1 - uint64(i)

		// Sign it
		ed := new(transactions.ED25519Sig)
		ed.PublicKey = m.Key[32:]
		err = ed.Sign(synthSig.Nonce, m.Key, txid[:])
		if err != nil {
			return err
		}

		// Add it to the list
		synthSig.Signature = ed.Signature
		body.Transactions = append(body.Transactions, synthSig)
	}

	// Construct the signature transaction
	tx := new(transactions.GenTransaction)
	tx.SigInfo = new(transactions.SignatureInfo)
	tx.SigInfo.URL = protocol.ACME
	tx.SigInfo.KeyPageIndex = 0
	tx.Transaction, err = body.MarshalBinary()
	if err != nil {
		return err
	}
	tx.SigInfo.KeyPageHeight = 1
	tx.SigInfo.Nonce, err = m.nextSynthCount()
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

	// Send it
	go m.Local.BroadcastTxAsync(context.Background(), data)
	return nil
}

// sendSynthTxns sends signed synthetic transactions from previous blocks.
func (m *Executor) sendSynthTxns() ([]abci.SynthTxnReference, error) {
	// Get the signatures from the last block
	sigs, err := m.DB.GetSynthTxnSigs()
	if err != nil {
		return nil, err
	}

	// Is there anything to send?
	if len(sigs) == 0 {
		return nil, nil
	}

	// Array for synth TXN references
	refs := make([]abci.SynthTxnReference, 0, len(sigs))

	// Process all the transactions
	for _, sig := range sigs {
		// Load the pending transaction object
		obj, err := m.DB.GetSynthTxn(sig.Txid)
		if err != nil {
			return nil, err
		}

		// Unmarshal it
		state := new(state.PendingTransaction)
		err = obj.As(state)
		if err != nil {
			return nil, err
		}

		// Convert it back to a transaction
		tx := state.Restore()

		// Add the signature
		tx.Signature = append(tx.Signature, &transactions.ED25519Sig{
			Nonce:     sig.Nonce,
			PublicKey: sig.PublicKey,
			Signature: sig.Signature,
		})

		// Marshal the transaction
		raw, err := tx.Marshal()
		if err != nil {
			return nil, err
		}

		// Parse the URL
		u, err := url.Parse(tx.SigInfo.URL)
		if err != nil {
			return nil, err
		}

		// Add it to the batch
		m.logger.Debug("Sending synth txn", "actor", u.String(), "txid", logging.AsHex(tx.TransactionHash()))
		m.dispatcher.BroadcastTxAsync(context.Background(), u, raw)

		// Delete the signature
		m.dbTx.DeleteSynthTxnSig(sig.Txid)

		// Add the synthetic transaction reference
		var ref abci.SynthTxnReference
		ref.Type = uint64(tx.TransactionType())
		ref.Url = tx.SigInfo.URL
		ref.TxRef = sha256.Sum256(raw)
		copy(ref.Hash[:], tx.TransactionHash())
		refs = append(refs, ref)
	}

	return refs, nil
}
