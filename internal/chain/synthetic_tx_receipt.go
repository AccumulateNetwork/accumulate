package chain

import (
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

	st.logger.Debug("received SyntheticReceipt from", body.Source.URL(), "for tx", logging.AsHex(body.SynthTxHash))
	st.UpdateStatus(body.SynthTxHash[:], body.Status)

	return nil, nil
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
	return txt.IsSynthetic()
}

// CreateSynthReceipt creates a receipt used to return the status of synthetic transactions to its sender
func CreateSynthReceipt(transaction *protocol.Transaction, status *protocol.TransactionStatus) (*url.URL, *protocol.SyntheticReceipt) {
	swo, ok := transaction.Body.(protocol.SynthTxnWithOrigin)
	if !ok {
		panic(fmt.Errorf("transcation type %v does not embed a SyntheticOrigin", transaction.Body.Type()))
	}

	cause, source := swo.GetSyntheticOrigin()
	sr := new(protocol.SyntheticReceipt)
	sr.SetSyntheticOrigin(cause, transaction.Header.Principal)
	sr.SynthTxHash = *(*[32]byte)(transaction.GetHash())
	sr.Status = status
	return source, sr
}
