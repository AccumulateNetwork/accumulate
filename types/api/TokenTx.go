package api

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/smt/common"

	"github.com/AccumulateNetwork/accumulated/types/proto"

	"github.com/AccumulateNetwork/accumulated/types"
)

type TokenTx struct {
	Hash types.Bytes32    `json:"hash,omitempty" form:"hash" query:"hash" validate:"required"` //,hexadecimal"`
	From types.UrlChain   `json:"from" form:"from" query:"from" validate:"required"`
	To   []*TokenTxOutput `json:"to" form:"to" query:"to" validate:"required"`
	Meta json.RawMessage  `json:"meta,omitempty" form:"meta" query:"meta" validate:"required"`
}

type TokenTxRequest struct {
	Hash types.String `json:"hash" form:"hash" query:"hash" validate:"required"` //,hexadecimal"`
	From types.String `json:"from" form:"from" query:"from" validate:"required"`
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

	var vi [8]byte
	_ = binary.PutVarint(vi[:], int64(proto.AccInstruction_Synthetic_Token_Transaction))

	buffer.Write(vi[:])

	data, err := t.From.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("error marshaling From %s,%v", t.From, err)
	}
	buffer.Write(data)

	_ = binary.PutVarint(vi[:], int64(len(t.To)))
	buffer.Write(vi[:])
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

// UnmarshalBinary deserialize the token transaction
func (t *TokenTx) UnmarshalBinary(data []byte) (err error) {
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

	if txType != uint64(proto.AccInstruction_Synthetic_Token_Transaction) {
		return fmt.Errorf("invalid transaction type, expecting TokenTx")
	}

	i := 1

	err = t.From.UnmarshalBinary(data[i:])
	if err != nil {
		return fmt.Errorf("unable to unmarshal FromUrl in transaction, %v", err)
	}

	i += t.From.Size(nil)
	if length < i {
		return fmt.Errorf("insufficient data to obtain number of token tx outputs for transaction")
	}

	toLen, n := binary.Varint(data[i:])
	i += n

	if length < i {
		return fmt.Errorf("insufficient data to unmarshal token tx outputs for transaction")
	}

	t.To = make([]*TokenTxOutput, toLen)
	for j := int64(0); j < toLen; j++ {
		txOut := &TokenTxOutput{}
		err := txOut.UnmarshalBinary(data[i:])
		i += txOut.Size()
		if err != nil {
			return fmt.Errorf("unable to unmarshal token tx output for index %d", i)
		}
		t.To[i] = txOut
	}

	if length < i {
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
