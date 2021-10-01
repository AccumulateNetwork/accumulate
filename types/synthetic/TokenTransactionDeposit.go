package synthetic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/AccumulateNetwork/accumulated/smt/common"
	"github.com/AccumulateNetwork/accumulated/types"
)

type TokenTransactionDeposit struct {
	Header
	DepositAmount big.Int          `json:"amount" form:"amount" query:"amount" validate:"gt=0"`
	TokenUrl      types.String     `json:"tokenURL" form:"tokenURL" query:"tokenURL" validate:"required,uri"`
	Metadata      *json.RawMessage `json:"meta,omitempty" form:"meta" query:"meta" validate:"required"`
}

func (tx *TokenTransactionDeposit) SetDeposit(tokenUrl *types.String, amt *big.Int) error {

	if amt == nil {
		return fmt.Errorf("no deposito amount specified")
	}

	if amt.Sign() <= 0 {
		return fmt.Errorf("deposit amount must be greater than 0")
	}

	tx.TokenUrl = *tokenUrl
	tx.DepositAmount.Set(amt)

	return nil
}

func NewTokenTransactionDeposit(txId types.Bytes, from *types.String, to *types.String) *TokenTransactionDeposit {
	tx := TokenTransactionDeposit{}
	tx.SetHeader(txId, from, to)
	return &tx
}

func (tx *TokenTransactionDeposit) Valid() error {

	if err := tx.Header.Valid(); err != nil {
		return err
	}

	if tx.DepositAmount.Sign() <= 0 {
		return fmt.Errorf("invalid deposit amount for token transaction deposit, amount must be greater than zero")
	}

	if len(tx.TokenUrl) == 0 {
		return fmt.Errorf("invalid tokenUrl set for token transaction deposit")
	}

	return nil
}

func (tx *TokenTransactionDeposit) MarshalBinary() ([]byte, error) {
	err := tx.Valid()
	if err != nil {
		return nil, err
	}
	var md []byte
	if tx.Metadata != nil {
		bmd := types.Bytes(*tx.Metadata)
		md, err = bmd.MarshalBinary()
		if err != nil {
			return nil, err
		}
	}

	var ret bytes.Buffer

	ret.Write(common.Uint64Bytes(types.TxTypeSyntheticTokenDeposit.AsUint64()))

	data, err := tx.Header.MarshalBinary()
	if err != nil {
		return nil, err
	}

	ret.Write(data)

	b := types.Bytes(tx.DepositAmount.Bytes())
	ret.WriteByte(byte(len(b)))
	ret.Write(b)

	data, err = tx.TokenUrl.MarshalBinary()
	if err != nil {
		return nil, err
	}
	ret.Write(data)

	if len(md) > 0 {
		ret.Write(md)
	}

	return ret.Bytes(), nil
}

func (tx *TokenTransactionDeposit) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if rErr := recover(); rErr != nil {
			err = fmt.Errorf("error marshaling Token Transaction Deposit %v", rErr)
		}
	}()

	length := len(data)
	if length < 2 {
		return fmt.Errorf("insufficient data to unmarshal binary for TokenTransactionDeposit")
	}

	//compare the type to make sure it is a synthetic tx.
	txType, data := common.BytesUint64(data)

	if txType != uint64(types.TxTypeSyntheticTokenDeposit) {
		return fmt.Errorf("invalid transaction type, expecting %s, but received %s",
			types.TxTypeSyntheticTokenDeposit.Name(), types.TxType(txType).Name())
	}

	err = tx.Header.UnmarshalBinary(data)
	if err != nil {
		return err
	}

	i := tx.Header.Size()

	if length < i {
		return fmt.Errorf("unable to unmarshal binary after token transaction deposit header")
	}
	l := i + int(data[i]) + 1
	if length < l {
		return fmt.Errorf("unable to unmarshal binary for deposit amount")
	}

	tx.DepositAmount.SetBytes(data[i+1 : l])
	i = l

	if length < i {
		return fmt.Errorf("unable to unmarshal binary before token url")
	}

	err = tx.TokenUrl.UnmarshalBinary(data[i:])
	if err != nil {
		return err
	}

	i += tx.TokenUrl.Size(nil)

	//if we still have stuff left over then it is the json raw message
	if i < length {
		var b types.Bytes
		err = b.UnmarshalBinary(data[i:])
		if err != nil {
			return err
		}
		if len(b) > 0 {
			tx.Metadata = &json.RawMessage{}
			copy(*tx.Metadata, b)
		}
	}

	return nil
}
