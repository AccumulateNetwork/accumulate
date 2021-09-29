package api

import (
	"bytes"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/smt/common"
	"github.com/AccumulateNetwork/accumulated/types"
)

type TokenAccount struct {
	URL      types.String `json:"url" form:"url" query:"url" validate:"required"`
	TokenURL types.String `json:"tokenURL" form:"tokenURL" query:"tokenURL" validate:"required,uri"`
}

func NewTokenAccount(accountURL types.String, issuingTokenURL types.String) *TokenAccount {
	tcc := &TokenAccount{URL: accountURL, TokenURL: issuingTokenURL}
	return tcc
}

func (t *TokenAccount) MarshalBinary() (data []byte, err error) {
	var buffer bytes.Buffer

	buffer.Write(common.Int64Bytes(int64(types.TxTypeTokenAccountCreate)))

	data, err = t.URL.MarshalBinary()
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

func (t *TokenAccount) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("error marshaling Pending Transaction State %v", err)
		}
	}()

	txType, data := common.BytesInt64(data)

	if txType != int64(types.TxTypeTokenAccountCreate) {
		return fmt.Errorf("invalid transaction type, expecting TokenAccount")
	}

	err = t.URL.UnmarshalBinary(data)
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
