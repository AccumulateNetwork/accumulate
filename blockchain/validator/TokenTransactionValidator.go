package validator

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	pb "github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
	cfg "github.com/tendermint/tendermint/config"
	"math/big"
	"time"

	"github.com/Factom-Asset-Tokens/factom"
	"github.com/Factom-Asset-Tokens/factom/fat"
	"github.com/Factom-Asset-Tokens/factom/fat103"
	"github.com/Factom-Asset-Tokens/factom/jsonlen"
	"github.com/Factom-Asset-Tokens/fatd/fat0"
)

//This follows the FAT-0 Specification for transactions

const Type = fat.TypeFAT0

type TokenTransactionValidator struct {
	ValidatorContext
}

/*
{
"identity-chain-path" : "RedWagon/MyAccTokens",
"payload" : {
   "send:": 150,
   "outputs:": {
      "GreenRock/Acc": 150
   },
   "metadata": {"memo": "thanks for dinner!"}
},
}
*/

//transactions are just accounts with balances on a given token chain
//what transaction types should be supported?
type TokenTransaction struct {
}

func (tx *TokenTransaction) MarshalBinary() ([]byte, error) {
	return nil, nil
}

func (tx *TokenTransaction) UnmarshalBinary(data []byte) error {

	return nil
}

func NewTokenTransactionValidator() *TokenTransactionValidator {
	v := TokenTransactionValidator{}

	//deprecate chainid in this context.. has no meaning.
	chainid := "0000000000000000000000000000000000000000000000000000000000000A75"

	v.SetInfo(chainid, "token-transaction", pb.AccInstruction_Token_Transaction)
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}

func (v *TokenTransactionValidator) canTransact(currentstate *StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) (*state.IdentityState, *state.TokenAccountState, *types.TokenTransaction, error) {

	//extract the identity state i.e. get the current keys... todo: does signature checking go here or can it be done before here?
	//

	ids := state.IdentityState{}
	err := ids.UnmarshalBinary(currentstate.IdentityState.Entry)
	if err != nil {
		return nil, nil, nil, err
	}

	tas := state.TokenAccountState{}
	err = tas.UnmarshalBinary(currentstate.ChainState.Entry)
	if err != nil {
		return nil, nil, nil, err
	}

	var tx types.TokenTransaction
	err = json.Unmarshal(data, &tx)

	if err != nil {
		return nil, nil, nil, err
	}
	//now check to see if we can transact
	//really only need to provide one input...

	amt := big.NewInt(0)
	for _, val := range tx.Output {
		amt.Add(amt, val)
	}

	if tx.TransferAmount.Cmp(amt) != 0 {
		return nil, nil, nil, fmt.Errorf("Transfer amount (%s) doesn't equal sum of the outputs (%s)",
			tx.TransferAmount.String(), amt.String())
	}

	if tas.Balance().Cmp(&tx.TransferAmount) < 0 {
		///insufficient balance
		return nil, nil, nil, fmt.Errorf("Insufficient balance")
	}
	return &ids, &tas, &tx, nil
}

func (v *TokenTransactionValidator) Check(currentstate *StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) error {
	_, _, _, err := v.canTransact(currentstate, identitychain, chainid, p1, p2, data)
	return err
}

func (v *TokenTransactionValidator) Initialize(config *cfg.Config) error {
	return nil
}

func (v *TokenTransactionValidator) BeginBlock(height int64, time *time.Time) error {
	v.lastHeight = v.currentHeight
	v.lastTime = v.currentTime
	v.currentHeight = height
	v.currentTime = *time

	return nil
}

func (v *TokenTransactionValidator) Validate(currentstate *StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) (*ResponseValidateTX, error) {

	//need to do everything done in "check" and also create a synthetic transaction to add tokens.
	_, _, tx, err := v.canTransact(currentstate, identitychain, chainid, p1, p2, data)

	if err != nil {
		return nil, err
	}

	ret := ResponseValidateTX{}
	ret.Submissions = make([]pb.Submission, 1)

	count := 0
	for outputaddr, val := range tx.Output {
		sub := pb.Submission{}
		adi, chainpath, err := types.ParseIdentityChainPath(outputaddr)
		if err != nil {
			return nil, err
		}
		idchain := types.GetIdentityChainFromIdentity(adi)
		if idchain == nil {
			return nil, fmt.Errorf("Invalid identity chain for %s", adi)
		}
		sub.Identitychain = idchain[:]

		chainid := types.GetChainIdFromChainPath(chainpath)

		sub.Chainid = chainid[:]

		sub.Instruction = pb.AccInstruction_Token_Deposit

		ret.Submissions[count] = sub
	}

	//where is this being routed to?
	//send to synth tx chain validator
	//chainid + 1
	ret.Submissions[1] = pb.Submission{
		Identitychain: identitychain, //should this be set externally?
		//Type:          GetTypeIdFromName("synthetic_transaction"), //should this get set externally?
		Instruction: pb.AccInstruction_Token_Transaction,
		Chainid:     chainid, //need a chain id of where you are going...  chainid + 1
		Param1:      0,
		Param2:      0,
		Data:        data, //need to make the data what it should be for atk
	}

	return nil, nil
	//return &pb.Submission{}, nil
}

func (v *TokenTransactionValidator) EndBlock(mdroot []byte) error {
	//copy(v.mdroot[:], mdroot[:])
	//don't think this serves a purpose???
	return nil
}

// Transaction represents a fat0 transaction, which can be a normal account
// transaction or a coinbase transaction depending on the Inputs and the
// RCD/signature pair.
type Transaction struct {
	Inputs  fat0.AddressAmountMap `json:"inputs"`
	Outputs fat0.AddressAmountMap `json:"outputs"`

	Metadata json.RawMessage `json:"metadata,omitempty"`

	Entry factom.Entry `json:"-"`
}

func NewTransaction(e *factom.Entry, idKey []byte) (Transaction, error) {
	var t Transaction
	if err := t.UnmarshalJSON(e.Content); err != nil {
		return t, err
	}

	if t.Inputs.Sum() != t.Outputs.Sum() {
		return t, fmt.Errorf("sum(inputs) != sum(outputs)")
	}

	var expected map[factom.Bytes32]struct{}
	// Coinbase transactions must only have one input.
	if t.IsCoinbase() {
		if len(t.Inputs) != 1 {
			return t, fmt.Errorf("invalid coinbase transaction")
		}

		k := factom.Bytes32{}
		copy(k[:], idKey)

		expected = map[factom.Bytes32]struct{}{k: struct{}{}}
	} else {
		expected = make(map[factom.Bytes32]struct{}, len(t.Inputs))
		for adr := range t.Inputs {
			expected[factom.Bytes32(adr)] = struct{}{}
		}
	}

	if err := fat103.Validate(*e, expected); err != nil {
		return t, err
	}

	t.Entry = *e

	return t, nil
}

func (t *Transaction) UnmarshalJSON(data []byte) error {
	data = jsonlen.Compact(data)
	var tRaw struct {
		Inputs   json.RawMessage `json:"inputs"`
		Outputs  json.RawMessage `json:"outputs"`
		Metadata json.RawMessage `json:"metadata,omitempty"`
	}
	if err := json.Unmarshal(data, &tRaw); err != nil {
		return fmt.Errorf("%T: %w", t, err)
	}
	if err := t.Inputs.UnmarshalJSON(tRaw.Inputs); err != nil {
		return fmt.Errorf("%T.Inputs: %w", t, err)
	}
	if err := t.Outputs.UnmarshalJSON(tRaw.Outputs); err != nil {
		return fmt.Errorf("%T.Outputs: %w", t, err)
	}
	t.Metadata = tRaw.Metadata

	expectedJSONLen := len(`{"inputs":,"outputs":}`) +
		len(tRaw.Inputs) + len(tRaw.Outputs)
	if tRaw.Metadata != nil {
		expectedJSONLen += len(`,"metadata":`) + len(tRaw.Metadata)
	}
	if expectedJSONLen != len(data) {
		return fmt.Errorf("%T: unexpected JSON length", t)
	}

	return nil
}

func (t Transaction) IsCoinbase() bool {
	_, ok := t.Inputs[fat.Coinbase()]
	return ok
}

func (t Transaction) String() string {
	data, err := json.Marshal(t)
	if err != nil {
		return err.Error()
	}
	return string(data)
}

func (t Transaction) Sign(signingSet ...factom.RCDSigner) (factom.Entry, error) {
	e := t.Entry
	content, err := json.Marshal(t)
	if err != nil {
		return e, err
	}
	e.Content = content
	return fat103.Sign(e, signingSet...), nil
}
