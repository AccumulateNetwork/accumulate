package validator

import (
	"encoding/json"
	"fmt"
	cfg "github.com/tendermint/tendermint/config"
	"time"

	"github.com/Factom-Asset-Tokens/factom"
	"github.com/Factom-Asset-Tokens/factom/fat"
	"github.com/Factom-Asset-Tokens/factom/fat103"
	"github.com/Factom-Asset-Tokens/factom/jsonlen"
	"github.com/Factom-Asset-Tokens/fatd/fat0"
)
//This follows the FAT-0 Specification for transactions

const Type = fat.TypeFAT0


type AtkTransactionValidator struct{
	ValidatorContext


}
/*
{
   "inputs:": {
      "RedWagon": 100,
      "BlueWagon": 50
   },
   "outputs:": {
      "GreenRock": 150
   },
   "metadata": {"memo": "thanks for dinner!"}
}
 */

//transactions are just accounts with balances on a given token chain
//what transaction types should be supported?
type AtkTransaction struct {

}

func (tx *AtkTransaction) MarshalBinary() ([]byte, error){
	return nil, nil
}

func (tx *AtkTransaction) UnmarshalBinary(data []byte) error{

	return nil
}

func NewAtkTransactionValidator() *AtkTransactionValidator {
	v := AtkTransactionValidator{}
	//need the chainid, then hash to get first 8 bytes to make the chainid.
	//by definition a chainid of a factoid block is
	//000000000000000000000000000000000000000000000000000000000000000f
	//the id will be 0x0000000f
	chainid := "0000000000000000000000000000000000000000000000000000000000000A75"
	v.SetInfo(chainid,"atk-transaction")
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}


func (v *AtkTransactionValidator) Check(currentstate *StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) error {
	e := factom.Entry{}
	e.UnmarshalBinary(data)
	_, err := NewTransaction(&e,chainid)
	if err != nil {
		return err
	}

	return nil
}
func (v *AtkTransactionValidator) Initialize(config *cfg.Config) error {
	return nil
}

func (v *AtkTransactionValidator) BeginBlock(height int64, time *time.Time) error {
	v.lastHeight = v.currentHeight
	v.lastTime = v.currentTime
	v.currentHeight = height
	v.currentTime = *time

	return nil
}

func (v *AtkTransactionValidator) Validate(currentstate *StateEntry, identitychain []byte,  chainid []byte, p1 uint64, p2 uint64, data []byte) (*ResponseValidateTX,error) {

	return nil, nil
	//return &pb.Submission{}, nil
}

func (v *AtkTransactionValidator) EndBlock(mdroot []byte) error  {
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
		copy(k[:],idKey)

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
