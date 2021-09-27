package chain

import (
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type Chain interface {
	// BeginBlock marks the beginning of a block.
	BeginBlock()

	// CheckTx partially validates the transaction.
	CheckTx(*state.StateEntry, *transactions.GenTransaction) error

	// DeliverTx fully validates the transaction.
	DeliverTx(*state.StateEntry, *transactions.GenTransaction) (*DeliverTxResult, error)

	// Commit commits the block.
	Commit()
}

type DeliverTxResult struct {
	StateData     map[types.Bytes32]types.Bytes  //acctypes.StateObject
	MainChainData map[types.Bytes32]types.Bytes  //stuff to store on pending chain.
	PendingData   map[types.Bytes32]types.Bytes  //stuff to store on pending chain.
	EventData     []byte                         //this should be events that need to get published
	Submissions   []*transactions.GenTransaction //this is a list of synthetic transactions
}

func (r *DeliverTxResult) AddMainChainData(chainid *types.Bytes32, data []byte) {
	if r.MainChainData == nil {
		r.MainChainData = make(map[types.Bytes32]types.Bytes)
	}
	r.MainChainData[*chainid] = data
}

func (r *DeliverTxResult) AddStateData(chainid *types.Bytes32, data []byte) {
	if r.StateData == nil {
		r.StateData = make(map[types.Bytes32]types.Bytes)
	}
	r.StateData[*chainid] = data
}

func (r *DeliverTxResult) AddPendingData(txId *types.Bytes32, data []byte) {
	if r.PendingData == nil {
		r.PendingData = make(map[types.Bytes32]types.Bytes)
	}
	r.PendingData[*txId] = data
}

func (r *DeliverTxResult) AddSyntheticTransaction(tx *transactions.GenTransaction) {
	r.Submissions = append(r.Submissions, tx)
}
