package types

import (
	"encoding/json"
	"fmt"
	"math/big"
)

type TokenTransaction struct {
	TransferAmount big.Int             `json:"transfer"`
	Output         map[string]*big.Int `json:"to-accounts"`
	Metadata       json.RawMessage     `json:"metadata,omitempty"`
}

func NewTokenTransaction(amt *big.Int, outputs *map[string]*big.Int) *TokenTransaction {
	tx := &TokenTransaction{}
	tx.SetTransferAmount(amt)

	if outputs != nil {
		for k, v := range *outputs {
			tx.AddToAccount(k, v)
		}
	}

	return tx
}

func (t *TokenTransaction) SetTransferAmount(amt *big.Int) error {
	if amt == nil {
		return fmt.Errorf("No Transfer Amount Specified")
	}
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
