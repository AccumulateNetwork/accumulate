package state

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"
)

type token struct {
	Chain
	Symbol    types.String     `json:"symbol" form:"symbol" query:"symbol" validate:"required,alphanum"`
	Precision types.Byte       `json:"precision" form:"precision" query:"precision" validate:"required,min=0,max=18"`
	Meta      *json.RawMessage `json:"meta,omitempty" form:"meta" query:"meta" validate:"optional"`
}

// Token implement the Entry interfaces for a token
type Token struct {
	Entry
	token
}

func NewToken(tokenUrl string) *Token {
	token := &Token{}
	token.SetHeader(types.String(tokenUrl), types.ChainTypeToken)
	return token
}

func (t *Token) GetChainUrl() string {
	return t.Chain.GetChainUrl()
}

func (t *Token) GetType() uint64 {
	return t.Chain.GetType()
}

func (t *Token) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	d, err := t.Chain.MarshalBinary()
	if err != nil {
		return nil, err
	}

	//marshal URL
	buffer.Write(d)

	//marshal Symbol
	d, err = t.Symbol.MarshalBinary()
	if err != nil {
		return nil, err
	}
	buffer.Write(d)

	//marshal precision
	buffer.WriteByte(byte(t.Precision))

	//if metadata exists marshal it
	if t.Meta != nil {
		var vi [8]byte
		l := binary.PutVarint(vi[:], int64(len(*t.Meta)))
		buffer.Write(vi[:l])
		buffer.Write(*t.Meta)
	}

	return buffer.Bytes(), nil
}

func (t *Token) UnmarshalBinary(data []byte) error {

	err := t.Chain.UnmarshalBinary(data)
	if err != nil {
		return err
	}

	i := t.GetHeaderSize()
	err = t.Symbol.UnmarshalBinary(data[i:])
	if err != nil {
		return err
	}

	i += t.Symbol.Size(nil)

	if i >= len(data) {
		return fmt.Errorf("unable to unmarshal data, precision not set")
	}
	t.Precision = types.Byte(data[i])
	i++

	if i < len(data) {
		v, l := binary.Varint(data[i:])
		i += l

		if len(data) < i+int(v) {
			return fmt.Errorf("unable to unmarshal data, metadata not set")
		}
		meta := json.RawMessage(data[i : i+int(v)])
		t.Meta = &meta
	}
	return nil
}
