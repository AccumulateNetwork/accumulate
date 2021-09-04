package api

import (
	"bytes"
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

func (t *TokenTx) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	data, err := t.From.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("error marshaling From %s,%v", t.From, err)
	}
	buffer.Write(data)

	for i, v := range t.To {
		data, err = v.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("error marshaling To[%d] %s,%v", i, v.URL, err)
		}
		buffer.Write(data)
	}

	a := types.Bytes(t.Meta)
	data, err = a.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("error marshalling meta data, %v", err)
	}
	buffer.Write(data)

	return buffer.Bytes(), nil
}

func (t *TokenTxOutput) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	data, err := t.URL.MarshalBinary()
	if err != nil {
		return nil, err
	}
	buffer.Write(data)

	data, err = t.Amount.MarshalBinary()
	if err != nil {
		return nil, err
	}
	buffer.Write(data)
	return buffer.Bytes(), nil
}

func (t *TokenTxOutput) UnmarshalBinary(data []byte) error {

	err := t.URL.UnmarshalBinary(data)
	if err != nil {
		return err
	}
	l := t.URL.Size(nil)
	if l > len(data) {
		return fmt.Errorf("insufficient data to unmarshal amount")
	}
	return t.Amount.UnmarshalBinary(data[l:])
}
