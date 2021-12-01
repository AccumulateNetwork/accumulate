package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/genesis"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type SyntheticGenesis struct{}

func (SyntheticGenesis) Type() types.TxType {
	return types.TxTypeSyntheticGenesis
}

func (SyntheticGenesis) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	err := tx.As(new(protocol.SyntheticGenesis))
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	for _, record := range genesis.OldBootstrapStates() {
		st.Update(record)
	}
	return nil
}

func (SyntheticGenesis) CheckTx(st *StateManager, tx *transactions.GenTransaction) error {
	return SyntheticGenesis{}.Validate(st, tx)
}

func (SyntheticGenesis) DeliverTx(st *StateManager, tx *transactions.GenTransaction) error {
	return SyntheticGenesis{}.Validate(st, tx)
}
