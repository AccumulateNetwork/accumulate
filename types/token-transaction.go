package types

import (
	"encoding/json"
	"fmt"
	"math/big"
)

//{"transfer":10000,"to-accounts":{"GreenRock/YourAtkTokens":10000}}
//need to formalize this in an object.
type TokenTransaction struct {
	TransferAmount big.Int             `json:"transfer,string"`
	Output         map[string]*big.Int `json:"to-accounts,string"`
	Metadata       json.RawMessage     `json:"metadata,omitempty"`
}

func (t *TokenTransaction) SetTransferAmount(amt *big.Int) error {
	if amt.Sign() < 0 {
		return fmt.Errorf("Invalid Transfer Amount")
	}
	t.TransferAmount.Set(amt)
	return nil
}

func (t *TokenTransaction) AddToAccount(aditokenpath string, amt *big.Int) error {
	if t.Output == nil {
		t.Output = make(map[string]*big.Int)
	}
	var toamt big.Int
	toamt.Set(amt)
	t.Output[aditokenpath] = &toamt
	return nil
}

func (t *TokenTransaction) SetMetadata(md *json.RawMessage) error {
	if md == nil {
		return fmt.Errorf("Invalid metadata")
	}
	copy(t.Metadata[:], (*md)[:])
	return nil
}
