package state

import (
	"bytes"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/types"
)

// Token implement the Entry interfaces for a token
type Token struct {
	ChainHeader
	Symbol        types.String `json:"symbol" form:"symbol" query:"symbol" validate:"required,alphanum"`
	Precision     types.Byte   `json:"precision" form:"precision" query:"precision" validate:"required,min=0,max=18"`
	PropertiesUrl types.String `json:"propertiesUrl" form:"propertiesUrl" query:"propertiesUrl" validate:"optional"`
}

func NewToken(tokenUrl string) *Token {
	token := &Token{}
	token.SetHeader(types.String(tokenUrl), types.ChainTypeToken)
	return token
}

func (t *Token) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	d, err := t.ChainHeader.MarshalBinary()
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

	d, err = t.PropertiesUrl.MarshalBinary()
	if err != nil {
		return nil, err
	}
	buffer.Write(d)

	return buffer.Bytes(), nil
}

func (t *Token) UnmarshalBinary(data []byte) error {

	err := t.ChainHeader.UnmarshalBinary(data)
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

	if i >= len(data) {
		return fmt.Errorf("unable to unmarshal data, propertiesUrl not set")
	}

	t.PropertiesUrl.UnmarshalBinary(data[i:])
	if err != nil {
		return err
	}

	return nil
}
