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

func (v *BlockValidator) CheckTx(st *state.StateEntry, tx *transactions.Transaction) error {
	// TODO shouldn't this be checking the subchains?
	return nil
}

func (v *BlockValidator) DeliverTx(st *state.StateEntry, tx *transactions.Transaction) (*DeliverTxResult, error) {
	txType := types.TxType(tx.TransactionType())

	var err error
	if err := tx.SetRoutingChainID(); err != nil {
		return nil, err
	}

	//If adiState doesn't exist, we will process by transaction instruction type
	if st.IdentityState == nil {
		//so the current state isn't defined, so we need to see if we need to create a token or anon chain.

		val := v.byInstr[txType]
		if val == nil {
			return nil, fmt.Errorf("unable to process identity with invalid instruction, %d", txType)
		}

		//valid actions for identity are to create an adi or create an account for anonymous address from synth transactions
		switch txType {
		case types.TxTypeSyntheticIdentityCreate: //a sponsor will generate the synth identity creation msg
			fallthrough
		case types.TxTypeSyntheticTokenDeposit: // for synth deposits, only anon addresses will be accepted
			return val.DeliverTx(st, tx)
		default:
			return nil, fmt.Errorf("invalid instruction issued for identity transaction, %d", txType)
		}
	}

	//since we have a valid adiState, we now need to look up the chain
	st.ChainState, _ = st.DB.GetCurrentEntry(tx.ChainID) //need the identity chain

	//If chain state doesn't exist, we will process by transaction instruction type
	if st.ChainState == nil {
		//we have no chain state, so we need to process by transaction type.
		val := v.byInstr[txType]
		if val == nil {
			return nil, fmt.Errorf("unable to process identity with invalid instruction, %d", txType)
		}

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
			return val.DeliverTx(st, tx)
		default:
			return nil, fmt.Errorf("invalid instruction issued for chain transaction, %d", txType)
		}
	}

	//if we get here, we have a valid chain state, so we need to pass it into the chain validator
	chain := state.Chain{}
	err = chain.UnmarshalBinary(st.ChainState.Entry)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal chain header for BlockValidator, %v", err)
	}

	//retrieve the validator based upon chain type
	val := v.byType[chain.Type]
	if val == nil {
		return nil, fmt.Errorf("cannot find validator for BlockValidationChain %s (err %v)", chain.ChainUrl, err)
	}

	//run the chain validator...
	return val.DeliverTx(st, tx)
}

func (*BlockValidator) Commit() {}
