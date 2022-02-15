package state

import "gitlab.com/accumulatenetwork/accumulate/protocol"

// NewPendingTransaction will create a new pending transaction from a general transaction
func NewPendingTransaction(env *protocol.Envelope) *PendingTransaction {
	ret := &PendingTransaction{}
	ret.Url = env.Transaction.Origin
	ret.Signature = env.Signatures
	ret.TransactionState = &TxState{}
	ret.TransactionState.SigInfo = &env.Transaction.TransactionHeader
	ret.TransactionState.Transaction = env.Transaction.Body
	copy(ret.TransactionState.TransactionHash[:], env.GetTxHash())
	return ret
}

// NewTransaction will take the transaction from the Pending Transaction, remove it and return a new pending
// transaction state and a new transaction state.  This is used for promtion of a transaction to the main chain
func NewTransaction(pending *PendingTransaction) (*Transaction, *PendingTransaction) {
	txState := &Transaction{}
	txState.Url = pending.Url
	txState.Transaction = pending.TransactionState.Transaction
	txState.SigInfo = pending.TransactionState.SigInfo
	txState.TxState.TransactionHash = pending.TransactionState.TransactionHash
	pending.TransactionState.Transaction = nil
	return txState, pending
}
