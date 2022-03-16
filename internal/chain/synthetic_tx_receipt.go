package chain

import (
	"bytes"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SyntheticReceipt struct{}

func (SyntheticReceipt) Type() protocol.TransactionType {
	return protocol.TransactionTypeSyntheticReceipt
}

/* === Receipt executor === */

func (SyntheticReceipt) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.SyntheticReceipt)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticReceipt), tx.Transaction.Body)
	}
	newStatus := new(protocol.TransactionStatus)
	newStatus.UnmarshalJSON(body.Status)

	// Load the transaction status & state
	synthTx := st.batch.Transaction(body.TxHash[:])
	curStatus, err := synthTx.GetStatus()
	if err != nil {
		st.logger.Error("received SyntheticReceipt from", body.Source.URL(), "but the status for tx", logging.AsHex(body.TxHash),
			"could not be retrieved:", err)
	}

	// Update the status when changed
	if curStatus == nil || !statusEqual(curStatus, newStatus) {
		st.logger.Debug("received SyntheticReceipt from", body.Source.URL(), "for tx ", logging.AsHex(body.TxHash),
			"and the status was changed. Updating status")
		synthTx.PutStatus(newStatus)
	} else {
		st.logger.Debug("received SyntheticReceipt from", body.Source.URL(), "for tx ", logging.AsHex(body.TxHash),
			"and the status was not changed. Skipping status update")
	}
	return nil, nil
}

// statusEqual compares TransactionStatus objects with the contents of TransactionResult. (The auto-gen code does result == result)
func statusEqual(v *protocol.TransactionStatus, u *protocol.TransactionStatus) bool {
	if !(v.Remote == u.Remote) {
		return false
	}
	if !(v.Delivered == u.Delivered) {
		return false
	}
	if !(v.Pending == u.Pending) {
		return false
	}
	if !(v.Code == u.Code) {
		return false
	}
	if !(v.Message == u.Message) {
		return false
	}
	if !(v.Result.Type() == u.Result.Type()) {
		return false
	}

	rv, _ := v.Result.MarshalBinary()
	ru, _ := u.Result.MarshalBinary()
	if bytes.Compare(rv, ru) != 0 {
		return false
	}

	return true
}

/* === Receipt generation === */

// NeedsReceipt selects which synth txs need / don't a receipt
func NeedsReceipt(txt protocol.TransactionType) bool {
	switch txt {
	case protocol.TransactionTypeSyntheticReceipt,
		protocol.TransactionTypeSyntheticAnchor,
		protocol.TransactionTypeSyntheticMirror,
		protocol.TransactionTypeSegWitDataEntry,
		protocol.TransactionTypeSignPending:
		return false
	}
	return true
}

// CreateReceipt creates a receipt used to return the status of synthetic transactions to its sender
func CreateReceipt(env *protocol.Envelope, status *protocol.TransactionStatus, nodeUrl *url.URL) (*protocol.SyntheticReceipt, *url.URL) {
	sr := new(protocol.SyntheticReceipt)
	sr.TxHash = *(*[32]byte)(env.GetTxHash())
	sr.Source = nodeUrl
	synthOrigin := getSyntheticOrigin(env.Transaction)
	sr.Cause = synthOrigin.Cause

	var err error
	sr.Status, err = status.MarshalJSON()
	if err != nil {
		panic(fmt.Errorf("can't marshal transaction status: %w", err))
	}

	return sr, synthOrigin.Source
}

// getSyntheticOrigin goes into the body of the synthetic transaction to extract the SyntheticOrigin we need for the receipt
func getSyntheticOrigin(tx *protocol.Transaction) protocol.SyntheticOrigin {
	switch tx.Type() {
	case protocol.TransactionTypeSyntheticCreateChain:
		body, ok := tx.Body.(*protocol.SyntheticCreateChain)
		if ok {
			return body.SyntheticOrigin
		}
	case protocol.TransactionTypeSyntheticWriteData:
		body, ok := tx.Body.(*protocol.SyntheticWriteData)
		if ok {
			return body.SyntheticOrigin
		}
	case protocol.TransactionTypeSyntheticDepositTokens:
		body, ok := tx.Body.(*protocol.SyntheticDepositTokens)
		if ok {
			return body.SyntheticOrigin
		}
	case protocol.TransactionTypeSyntheticDepositCredits:
		body, ok := tx.Body.(*protocol.SyntheticDepositCredits)
		if ok {
			return body.SyntheticOrigin
		}
	case protocol.TransactionTypeSyntheticBurnTokens:
		body, ok := tx.Body.(*protocol.SyntheticBurnTokens)
		if ok {
			return body.SyntheticOrigin
		}
	default:
		panic(fmt.Errorf("transcation type %v does not embed a SyntheticOrigin", tx.Type()))
	}
	return protocol.SyntheticOrigin{} // Unreachable
}
