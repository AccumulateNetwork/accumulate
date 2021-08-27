package api

import (
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
)

type TokenTx struct {
	Hash types.Bytes32    `json:"hash,omitempty" form:"hash" query:"hash" validate:"required"` //,hexadecimal"`
	From types.UrlChain   `json:"from" form:"from" query:"from" validate:"required"`
	To   []*TokenTxOutput `json:"to" form:"to" query:"to" validate:"required"`
	Meta json.RawMessage  `json:"meta,omitempty" form:"meta" query:"meta" validate:"required"`
}

type TokenTxOutput struct {
	URL    types.UrlChain `json:"url" form:"url" query:"url" validate:"required"`
	Amount types.Amount   `json:"amount" form:"amount" query:"amount" validate:"gt=0"`
}

func NewTokenTx(from types.UrlChain) *TokenTx {
	tx := &TokenTx{}
	tx.From = from

	return tx
}

func (t *TokenTx) AddToAccount(tourl types.UrlChain, amt *types.Amount) {
	txOut := TokenTxOutput{tourl, *amt}
	t.To = append(t.To, &txOut)
}

func (t *TokenTx) SetMetadata(md *json.RawMessage) error {
	if md == nil {
		return fmt.Errorf("invalid metadata")
	}
	copy(t.Meta[:], (*md)[:])
	return nil
}
