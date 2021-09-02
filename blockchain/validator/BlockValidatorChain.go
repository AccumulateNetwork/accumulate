package validator

import (
	"fmt"
	pb "github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
	cfg "github.com/tendermint/tendermint/config"
)

// BlockValidatorChain is the transaction entry point for all validation
type BlockValidatorChain struct {
	ValidatorContext
}

// NewBlockValidatorChain is the entry point for transactions it only knows how to handle
// adi types and anonymous types.
func NewBlockValidatorChain() *BlockValidatorChain {
	v := BlockValidatorChain{}

	//add chain validators (Adi, anon, accounts)
	v.addValidator(&NewAdiChain().ValidatorContext)
	v.addValidator(&NewAnonTokenChain().ValidatorContext)
	//v.addValidator(&NewAccountChain().ValidatorContext) <-- handles deposits and sends

	//add transaction validators for creation of adi, token, or account
	v.addValidator(&NewSyntheticIdentityStateCreateValidator().ValidatorContext)
	v.addValidator(&NewTokenIssuanceValidator().ValidatorContext)
	v.addValidator(&NewTokenChainCreateValidator().ValidatorContext)

	v.ValidatorContext.ValidatorInterface = &v
	return &v
}

func (v *BlockValidatorChain) Check(currentState *state.StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) error {
	return nil
}
func (v *BlockValidatorChain) Initialize(config *cfg.Config) error {
	return nil
}

func (v *BlockValidatorChain) Validate(currentState *state.StateEntry, sub *pb.Submission) (*ResponseValidateTX, error) {
	var err error

	//the state entry will be nil, anon addr, or adi state
	currentState.IdentityState, err = currentState.DB.GetCurrentState(sub.GetIdentitychain()) //need the identity chain

	//If adiState doesn't exist, we will process by transaction instruction type
	if err != nil {
		//so the current state isn't defined, so we need to see if we need to create a token or anon chain.
		val, err := v.getValidatorByIns(sub.Instruction)

		if err != nil {
			return nil, fmt.Errorf("unable to process identity with invalid instruction, %d", sub.Instruction)
		}

		//valid actions for identity are to create an adi or create an account for anonymous address
		switch sub.Instruction {
		case pb.AccInstruction_Identity_Creation:
			fallthrough
		case pb.AccInstruction_Synthetic_Token_Deposit: // for synth deposits, only anon addresses will be accepted
			return val.Validate(currentState, sub)
		default:
			return nil, fmt.Errorf("invalid instruction issued for identity transaction, %d", sub.Instruction)
		}
	}

	//since we have a valid adiState, we now need to look up the chain
	currentState.ChainState, err = currentState.DB.GetCurrentState(sub.GetChainid()) //need the identity chain

	//If chain state doesn't exist, we will process by transaction instruction type
	if err != nil {
		//we have no chain state, so we need to process by transaction type.
		val, err := v.getValidatorByIns(sub.Instruction)
		if err != nil {
			return nil, fmt.Errorf("unable to process identity with invalid instruction, %d", sub.Instruction)
		}

		//valid actions are to create account, token, scratch chain, or data chain
		switch sub.Instruction {
		case pb.AccInstruction_Scratch_Chain_Creation:
			fallthrough
		case pb.AccInstruction_Data_Chain_Creation:
			fallthrough
		case pb.AccInstruction_Token_Issue:
			fallthrough
		case pb.AccInstruction_Token_URL_Creation:
			return val.Validate(currentState, sub)
		default:
			return nil, fmt.Errorf("invalid instruction issued for chain transaction, %d", sub.Instruction)
		}
	}

	//if we get here, we have a valid chain state, so we need to pass it into the chain validator
	chain := state.Chain{}
	err = chain.UnmarshalBinary(currentState.ChainState.Entry)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal chain header for BlockValidatorChain, %v", err)
	}

	//retrieve the validator based upon chain type
	val, err := v.getValidatorByType(&chain.Type)
	if err != nil {
		return nil, fmt.Errorf("cannot find validator for BlockValidationChain %s (err %v)", chain.ChainUrl, err)
	}

	//run the chain validator...
	return val.Validate(currentState, sub)
}

func (v *BlockValidatorChain) EndBlock(mdroot []byte) error {
	return nil
}
