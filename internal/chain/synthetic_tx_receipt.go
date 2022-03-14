package chain

import (
	"fmt"

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

	newTxStatus := new(protocol.TransactionStatus)
	newTxStatus.UnmarshalJSON(body.Status)

	// Load the transaction
	synthTx := st.batch.Transaction(body.TxHash[:])
	synthTx.PutStatus(newTxStatus)

	return nil, nil
}

/* === Receipt generation === */

// CreateReceipt a receipt used to return the status of synthetic transactions to its senders
func CreateReceipt(env *protocol.Envelope, status *protocol.TransactionStatus) *protocol.SyntheticReceipt {
	sr := new(protocol.SyntheticReceipt)
	sr.TxHash = *(*[32]byte)(env.GetTxHash())

	var err error
	sr.Status, err = status.MarshalJSON()
	if err != nil {
		panic(fmt.Errorf("can't marshal transaction status: %w", err))
	}

	sr.Cause = *getCause(env.Transaction)
	return sr
}

// getCause gets the synth tx cause when it can. Only specific tx bodies contain that field
func getCause(tx *protocol.Transaction) *[32]byte {
	switch tx.Type() {
	case protocol.TransactionTypeSyntheticCreateChain:
		body, ok := tx.Body.(*protocol.SyntheticCreateChain)
		if ok {
			return &body.Cause
		}
	case protocol.TransactionTypeSyntheticWriteData:
		body, ok := tx.Body.(*protocol.SyntheticWriteData)
		if ok {
			return &body.Cause
		}
	case protocol.TransactionTypeSyntheticDepositTokens:
		body, ok := tx.Body.(*protocol.SyntheticDepositTokens)
		if ok {
			return &body.Cause
		}
	case protocol.TransactionTypeSyntheticDepositCredits:
		body, ok := tx.Body.(*protocol.SyntheticDepositCredits)
		if ok {
			return &body.Cause
		}
	case protocol.TransactionTypeSyntheticBurnTokens:
		body, ok := tx.Body.(*protocol.SyntheticBurnTokens)
		if ok {
			return &body.Cause
		}
	case protocol.TransactionTypeSegWitDataEntry:
		body, ok := tx.Body.(*protocol.SegWitDataEntry)
		if ok {
			return &body.Cause
		}
	}
	return nil
}
