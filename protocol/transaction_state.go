package protocol

import (
	"gitlab.com/accumulatenetwork/accumulate/smt/common"
)

func (tx *TransactionState) Restore() *Envelope {
	gtx := new(Envelope)
	gtx.Transaction = new(Transaction)
	gtx.Transaction.TransactionHeader = *tx.SigInfo
	gtx.Transaction.Body = tx.Transaction
	return gtx
}

func (tx *TransactionState) TxType() TransactionType {
	if tx.Transaction == nil {
		return TransactionTypeUnknown
	}
	transType, _ := common.BytesUint64(tx.Transaction)
	return TransactionType(transType)
}

func (tx *PendingTransactionState) Restore() *Envelope {
	gtx := new(Envelope)
	gtx.Transaction = new(Transaction)
	gtx.Signatures = tx.Signature
	gtx.Transaction.TransactionHeader = *tx.TransactionState.SigInfo
	gtx.Transaction.Body = tx.TransactionState.Transaction
	return gtx
}

func (is *TransactionState) TransactionHash() *[32]byte {
	tx := new(Transaction)
	tx.TransactionHeader = *is.SigInfo
	tx.Body = is.Transaction
	env := new(Envelope)
	env.Transaction = tx
	return (*[32]byte)(env.GetTxHash())
}
