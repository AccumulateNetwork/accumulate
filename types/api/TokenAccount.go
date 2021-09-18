package api

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/AccumulateNetwork/SMT/common"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/proto"
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
	var vi [8]byte
	_ = binary.PutVarint(vi[:], int64(proto.AccInstruction_Synthetic_Token_Transaction))

	buffer.Write(vi[:])

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

	length := len(data)
	if length < 2 {
		return fmt.Errorf("insufficient data to unmarshal binary for TokenTx")
	}

	txType, data := common.BytesUint64(data) //                                 Get the url

	if txType != uint64(proto.AccInstruction_Token_URL_Creation) {
		return fmt.Errorf("invalid transaction type, expecting TokenTx")
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
