package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"

	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

func NewBlockValidator() *validator {
	b := new(validator)
	b.add(IdentityCreate{})
	b.add(new(AnonToken))
	b.add(SynthIdentityCreate{})
	b.add(TokenIssuance{})
	b.add(TokenAccountCreate{})
	b.add(TokenTx{})
	b.add(SynthTokenAccountCreate{})
	b.add(SynthTokenDeposit{})
	return b
}

type validator struct {
	chainCreate map[types.TxType]Chain
	chainUpdate map[types.ChainType]Chain
}

var _ Validator = (*validator)(nil)

type createChain interface {
	createChain() types.TxType
	Chain
}

type updateChain interface {
	updateChain() types.ChainType
	Chain
}

func (v *validator) add(chain Chain) {
	if v.chainCreate == nil {
		v.chainCreate = map[types.TxType]Chain{}
		v.chainUpdate = map[types.ChainType]Chain{}
	}

	var used bool
	if chain, ok := chain.(createChain); ok {
		if _, ok := v.chainCreate[chain.createChain()]; ok {
			panic(fmt.Errorf("duplicate  create chain for %d", chain.createChain()))
		}
		v.chainCreate[chain.createChain()], used = chain, true
	}

	if chain, ok := chain.(updateChain); ok {
		if _, ok := v.chainUpdate[chain.updateChain()]; ok {
			panic(fmt.Errorf("duplicate identity create chain for %d", chain.updateChain()))
		}
		v.chainUpdate[chain.updateChain()], used = chain, true
	}

	if !used {
		panic(fmt.Errorf("unsupported chain type %T", chain))
	}
}

// BeginBlock will set block parameters
func (v *validator) BeginBlock() {
	for _, c := range v.chainCreate {
		c.BeginBlock()
	}
	for _, c := range v.chainUpdate {
		c.BeginBlock()
	}
}

func (v *validator) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	// TODO shouldn't this be checking the subchains?
	return nil
}

func (v *validator) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	txType := types.TxType(tx.TransactionType())

	if err := tx.SetRoutingChainID(); err != nil {
		return nil, err
	}

	chain := v.chainCreate[txType]
	if chain == nil {
		return nil, fmt.Errorf("cannot create identity: unsupported TX type: %d", txType)
	}
	return chain.DeliverTx(st, tx)
}

func (*validator) Commit() {}
