package chain

import (
	"encoding/hex"
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SyntheticReceipt struct{}

func (SyntheticReceipt) Type() protocol.TransactionType {
	return protocol.TransactionTypeSyntheticReceipt
}

func (SyntheticReceipt) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.SyntheticReceipt)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticReceipt), tx.Transaction.Body)
	}
	fmt.Printf("Synth transaction %s failed, cause: %s\n", hex.EncodeToString(body.TxHash), hex.EncodeToString(body.Cause[:])) // TODO REMOVE AFTER DEBUG
	return nil, nil
}

// SendReceipt is called by other synthetic transactions to report failure
func SendReceipt(st *StateManager, txHash []byte, cause [32]byte, err error) error {
	sr := new(protocol.SyntheticReceipt)
	sr.TxHash = txHash
	sr.Cause = cause
	if err != nil {
		sr.Reason = err.Error()
	}
	st.Submit(st.OriginUrl, sr)
	return err
}
