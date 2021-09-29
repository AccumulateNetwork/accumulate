package chain

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"

	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type chainTypeId uint64

type operations struct {
	byType  map[types.ChainType]operation
	byInstr map[types.TxType]operation
}

type operation interface {
	chainType() types.ChainType
	instruction() types.TxType

	BeginBlock()
	CheckTx(*state.StateEntry, *transactions.GenTransaction) error
	DeliverTx(*state.StateEntry, *transactions.GenTransaction) (*DeliverTxResult, error)
}

func (m *operations) add(chain operation) {
	typ, ins := chain.chainType(), chain.instruction()
	if m.byType == nil {
		m.byType = map[types.ChainType]operation{typ: chain}
		m.byInstr = map[types.TxType]operation{ins: chain}
		return
	}

	if _, ok := m.byInstr[ins]; ok {
		panic(fmt.Errorf("already have a subchain for instruction %d", ins))
	}

	// TODO If there's already an entry for the chain type, it will be
	// overwritten. That probably shouldn't happen.
	m.byType[typ] = chain
	m.byInstr[ins] = chain
}

func (m *operations) BeginBlock() {
	for _, c := range m.byInstr {
		c.BeginBlock()
	}
}
