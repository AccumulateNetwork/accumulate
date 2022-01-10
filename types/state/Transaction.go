package state

import (
	"github.com/AccumulateNetwork/accumulate/smt/common"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

// NewPendingTransaction will create a new pending transaction from a general transaction
func NewPendingTransaction(env *transactions.Envelope) *PendingTransaction {
	ret := &PendingTransaction{}
	ret.ChainHeader.SetHeader(types.String(env.Transaction.Origin.String()), types.ChainTypePendingTransaction)
	ret.Signature = env.Signatures
	ret.TransactionState = &TxState{}
	ret.TransactionState.SigInfo = &env.Transaction.Header
	ret.TransactionState.Transaction = env.Transaction.Body
	copy(ret.TransactionState.transactionHash[:], env.Transaction.Hash())
	return ret
}

// NewTransaction will take the transaction from the Pending Transaction, remove it and return a new pending
// transaction state and a new transaction state.  This is used for promtion of a transaction to the main chain
func NewTransaction(pending *PendingTransaction) (*Transaction, *PendingTransaction) {
	txState := &Transaction{}
	txState.ChainUrl = pending.ChainUrl
	txState.Type = types.ChainTypeTransaction
	txState.Transaction = pending.TransactionState.Transaction
	txState.SigInfo = pending.TransactionState.SigInfo
	txState.transactionHash = pending.TransactionState.transactionHash
	pending.TransactionState.Transaction = nil
	return txState, pending
}

func (tx *Transaction) Restore() *transactions.Envelope {
	gtx := new(transactions.Envelope)
	gtx.Transaction = new(transactions.Transaction)
	gtx.Transaction.Header = *tx.SigInfo
	gtx.Transaction.Body = tx.Transaction
	return gtx
}

func (tx *Transaction) TxType() types.TransactionType {
	if tx.Transaction == nil {
		return types.TxTypeUnknown
	}
	transType, _ := common.BytesUint64(tx.Transaction)
	return types.TxType(transType)
}

func (tx *PendingTransaction) Restore() *transactions.Envelope {
	gtx := new(transactions.Envelope)
	gtx.Transaction = new(transactions.Transaction)
	gtx.Signatures = tx.Signature
	gtx.Transaction.Header = *tx.TransactionState.SigInfo
	gtx.Transaction.Body = tx.TransactionState.Transaction
	return gtx
}

func (is *Transaction) TransactionHash() *types.Bytes32 {
	tx := new(transactions.Transaction)
	tx.Header = *is.SigInfo
	tx.Body = is.Transaction
	var txHash types.Bytes32
	copy(txHash[:], tx.Hash())
	return &txHash
}
