package api

import (
	"bytes"
	"encoding"
	"github.com/AccumulateNetwork/accumulated/types"
)

type TokenAccount struct {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
	URL      types.String `json:"url" form:"url" query:"url" validate:"required"`
	TokenURL types.String `json:"tokenURL" form:"tokenURL" query:"tokenURL" validate:"required,uri"`
}

func NewTokenAccount(accountURL types.String, issuingTokenURL types.String) *TokenAccount {
	tcc := &TokenAccount{URL: accountURL, TokenURL: issuingTokenURL}
	return tcc
}

func (t *TokenAccount) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer
	data, err := t.URL.MarshalBinary()
	if err != nil {
		return nil, err
	}
	buffer.Write(data)

	data, err = t.TokenURL.MarshalBinary()
	if err != nil {
		return nil, err
	}
	buffer.Write(data)

	return buffer.Bytes(), nil
}

func (t *TokenAccount) UnmarshalBinary(data []byte) error {

	err := t.URL.UnmarshalBinary(data)
	if err != nil {
		return err
	}

	i := t.URL.Size(nil)

	err = t.TokenURL.UnmarshalBinary(data[i:])
	if err != nil {
		return err
	}

	return nil
}
