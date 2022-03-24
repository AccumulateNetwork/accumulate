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
	return true
}

// CreateReceipt creates a receipt used to return the status of synthetic transactions to its sender
func CreateReceipt(env *protocol.Envelope, status *protocol.TransactionStatus, nodeUrl *url.URL) (*protocol.SyntheticReceipt, *url.URL) {
	synthOrigin := getSyntheticOrigin(env.Transaction)
	sr := new(protocol.SyntheticReceipt)
	sr.SetSyntheticOrigin(synthOrigin.Cause[:], nodeUrl)
	sr.SynthTxHash = *(*[32]byte)(env.GetTxHash())
	sr.Status = status

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
