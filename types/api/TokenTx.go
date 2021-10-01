package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/smt/common"
	"github.com/AccumulateNetwork/accumulated/types"
)

const MaxTokenTxOutputs = 100

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

func NewTokenTx(from types.String) *TokenTx {
	tx := &TokenTx{}
	tx.From = types.UrlChain{String: from}
	return tx
}

func (t *TokenTx) AddToAccount(toUrl types.String, amt *types.Amount) {
	txOut := TokenTxOutput{types.UrlChain{String: toUrl}, *amt}
	t.To = append(t.To, &txOut)
}

func (t *TokenTx) SetMetadata(md *json.RawMessage) error {
	if md == nil {
		return fmt.Errorf("invalid metadata")
	}
	copy(t.Meta[:], (*md)[:])
	return nil
}

// MarshalBinary serialize the token transaction
func (t *TokenTx) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(common.Int64Bytes(int64(types.TxTypeSyntheticTokenTx)))

	data, err := t.From.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("error marshaling From %s,%v", t.From, err)
	}
	buffer.Write(data)

	numOutputs := int64(len(t.To))
	if numOutputs > MaxTokenTxOutputs {
		return nil, fmt.Errorf("too many outputs for token transaction, please specify between 1 and %d outputs", MaxTokenTxOutputs)
	}
	if numOutputs < 1 {
		return nil, fmt.Errorf("insufficient token transaction outputs, please specify between 1 and %d outputs", MaxTokenTxOutputs)
	}
	buffer.Write(common.Int64Bytes(numOutputs))
	for i, v := range t.To {
		data, err = v.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("error marshaling To[%d] %s,%v", i, v.URL, err)
		}
		buffer.Write(data)
	}

	a := types.Bytes(t.Meta)
	if a != nil {
		data, err = a.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("error marshalling meta data, %v", err)
		}
		buffer.Write(data)
	}

	return buffer.Bytes(), nil
}

// UnmarshalBinary deserialize the token transaction
func (t *TokenTx) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("error marshaling TokenTx State %v", err)
		}
	}()

	txType, data := common.BytesInt64(data) // get the type

	if txType != int64(types.TxTypeSyntheticTokenTx) {
		return fmt.Errorf("invalid transaction type, expecting TokenTx")
	}

	err = t.From.UnmarshalBinary(data)
	if err != nil {
		return fmt.Errorf("unable to unmarshal FromUrl in transaction, %v", err)
	}

	//get the length of the outputs
	toLen, data := common.BytesInt64(data[t.From.Size(nil):])

	if toLen > 100 || toLen < 1 {
		return fmt.Errorf("invalid number of outputs for transaction")
	}
	t.To = make([]*TokenTxOutput, toLen)
	i := 0
	for j := int64(0); j < toLen; j++ {
		txOut := &TokenTxOutput{}
		err := txOut.UnmarshalBinary(data[i:])
		if err != nil {
			return fmt.Errorf("unable to unmarshal token tx output for index %d", j)
		}
		i += txOut.Size()
		t.To[j] = txOut
	}

	if len(data) > i {
		//we have metadata
		b := types.Bytes{}
		err := b.UnmarshalBinary(data[i:])
		if err != nil {
			return fmt.Errorf("unable to unmarshal binary for meta data of transaction token tx")
		}
		t.Meta = b.Bytes()
	}

	return nil
}

// Size return the size of the Token Tx output
func (t *TokenTxOutput) Size() int {
	return t.URL.Size(nil) + t.Amount.Size()
}

// MarshalBinary serialize the token tx output
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

// UnmarshalBinary deserialize the token tx output
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
