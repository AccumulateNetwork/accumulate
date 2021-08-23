package types

import (
	"encoding/json"
	"fmt"
)

type TokenTx struct {
	Hash Bytes32          `json:"hash" form:"hash" query:"hash" validate:"required"` //,hexadecimal"`
	From UrlChain         `json:"from" form:"from" query:"from" validate:"required"`
	To   []*TokenTxOutput `json:"to" form:"to" query:"to" validate:"required"`
	Meta json.RawMessage  `json:"meta" form:"meta" query:"meta" validate:"required"`
}

type TokenTxOutput struct {
	URL    UrlChain `json:"url" form:"url" query:"url" validate:"required"`
	Amount Amount   `json:"amount" form:"amount" query:"amount" validate:"gt=0"`
}

func NewTokenTx(from UrlChain) *TokenTx {
	tx := &TokenTx{}
	tx.From = from

	return tx
}

func (t *TokenTx) AddToAccount(tourl UrlChain, amt *Amount) {
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
