package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"

	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type BlockValidator struct {
	operations
}

var _ Chain = (*BlockValidator)(nil)

func NewBlockValidator() *BlockValidator {
	// We will be moving towards the account chain validator for token
	// transactions and eventually data tx
	b := new(BlockValidator)
	b.add(ADI{})
	b.add(new(AnonToken))
	b.add(SynIdentityCreate{})
	b.add(TokenIssuance{})
	b.add(TokenChainCreate{})
	b.add(TokenTx{})
	return b
}

// BeginBlock will set block parameters
func (v *BlockValidator) BeginBlock() {
	v.operations.BeginBlock()
}

func (v *BlockValidator) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	// TODO shouldn't this be checking the subchains?
	return nil
}

func (v *BlockValidator) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	txType := types.TxType(tx.TransactionType())

	if err := tx.SetRoutingChainID(); err != nil {
		return nil, err
	}

	var operation operation
	if st.AdiState == nil || st.ChainHeader == nil {
		operation = v.byInstr[txType]
		if operation == nil {
			return nil, fmt.Errorf("invalid TX type: %d", txType)
		}
	}

	// Chain state exists, but the chain's identity does not
	if st.AdiState == nil {
		//valid actions for identity are to create an adi or create an account for anonymous address from synth transactions
		switch txType {
		case types.TxTypeSyntheticIdentityCreate: //a sponsor will generate the synth identity creation msg
			fallthrough
		case types.TxTypeSyntheticTokenDeposit: // for synth deposits, only anon addresses will be accepted
			return operation.DeliverTx(st, tx)
		default:
			return nil, fmt.Errorf("invalid instruction issued for identity transaction, %d", txType)
		}
	}

	// No chain state exists for tx.ChainID
	if st.ChainHeader == nil {
		//valid instruction actions are to create account, token, identity, scratch chain, or data chain
		switch txType {
		case types.TxTypeIdentityCreate:
			fallthrough
		case types.TxTypeScratchChainCreate:
			fallthrough
		case types.TxTypeDataChainCreate:
			fallthrough
		case types.TxTypeTokenCreate:
			fallthrough
		case types.TxTypeTokenAccountCreate:
			return operation.DeliverTx(st, tx)
		default:
			return nil, fmt.Errorf("invalid instruction issued for chain transaction, %d", txType)
		}
	}

	operation = v.byType[st.ChainHeader.Type]
	if operation == nil {
		return nil, fmt.Errorf("invalid chain type: %d", st.ChainHeader.Type)
	}

	return operation.DeliverTx(st, tx)
}

func (*BlockValidator) Commit() {}
