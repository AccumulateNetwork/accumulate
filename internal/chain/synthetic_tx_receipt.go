package chain

import (
	"fmt"
	"math/big"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SyntheticReceipt struct{}

func (SyntheticReceipt) Type() protocol.TransactionType {
	return protocol.TransactionTypeSyntheticReceipt
}

func (SyntheticReceipt) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (SyntheticReceipt{}).Validate(st, tx)
}

/* === Receipt executor === */

func (SyntheticReceipt) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.SyntheticReceipt)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticReceipt), tx.Transaction.Body)
	}
	if body.Status.Code != 0 {
		txn, err := st.batch.Transaction(body.Cause[:]).GetState()
		if err != nil {
			return nil, err
		}

		if txn.Transaction.Type() == protocol.TransactionTypeSendTokens {
			var signer protocol.Signer
			obj := st.batch.Account(body.Status.Initiator)
			err = obj.GetStateAs(&signer)
			if err != nil {
				return nil, fmt.Errorf("load initial signer: %w", err)
			}
			var account protocol.AccountWithTokens
			err := st.batch.Account(txn.Transaction.Header.Principal).GetStateAs(&account)
			if err != nil {
				return nil, err
			}
			token := txn.Transaction.Body.(*protocol.SendTokens)
			var refund big.Int
			for _, entry := range token.To {
				if entry.Url.Equal(body.Source) {
					refund.Add(&refund, &entry.Amount)
					break
				}
			}
			account.CreditTokens(&refund)
			err = st.Update(account)
			if err != nil {
				return nil, fmt.Errorf("failed to update %v: %v", account.GetUrl(), err)
			}
		}

		paid, err := protocol.ComputeTransactionFee(txn.Transaction)
		if err != nil {
			return nil, fmt.Errorf("compute fee error: %w", err)
		}
		status, err := st.batch.Transaction(body.Cause[:]).GetStatus()
		if err != nil {
			return nil, err
		}
		if paid > protocol.FeeFailedMaximum {
			var signer protocol.Signer
			obj := st.batch.Account(status.Initiator)
			err = obj.GetStateAs(&signer)
			if err != nil {
				return nil, fmt.Errorf("load initial signer: %w", err)
			}
			refundAmount := uint64(paid) - protocol.FeeFailedMaximum.AsUInt64()
			if txn.Transaction.Header.Principal.LocalTo(signer.GetUrl()) {
				signer.CreditCredits(refundAmount)
				err := st.Update(signer)
				if err != nil {
					return nil, fmt.Errorf("failed to update %v: %v", signer.GetUrl(), err)
				}
			} else {
				deposit := new(protocol.SyntheticDepositCredits)
				deposit.Amount = refundAmount
				st.Submit(status.Initiator, deposit)
			}
		}
	}
	st.logger.Debug("received SyntheticReceipt, updating status", "from", body.Source.URL(), "for tx", logging.AsHex(body.SynthTxHash))
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
		protocol.TransactionTypeSyntheticForwardTransaction,
		protocol.TransactionTypeRemote:
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
