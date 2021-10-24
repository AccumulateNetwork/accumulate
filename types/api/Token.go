package api

import (
	"bytes"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/smt/common"
	"github.com/AccumulateNetwork/accumulated/types"
)

type TokenCirculationMode int

type Token struct {
	URL           types.String `json:"url" form:"url" query:"url" validate:"required"`
	Symbol        types.String `json:"symbol" form:"symbol" query:"symbol" validate:"required,alphanum"`
	Precision     types.Byte   `json:"precision" form:"precision" query:"precision" validate:"required,min=0,max=18"`
	PropertiesUrl types.String `json:"propertiesUrl" form:"propertiesUrl" query:"propertiesUrl"`
}

func NewToken(url string, symbol string, precision byte, propertiesUrl string) *Token {
	t := &Token{URL: types.String(url), Symbol: types.String(symbol),
		Precision: types.Byte(precision), PropertiesUrl: types.String(propertiesUrl)}
	return t
}

func (t *Token) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(common.Uint64Bytes(types.TxTypeTokenCreate.AsUint64()))

	//marshal URL
	d, err := t.URL.MarshalBinary()
	if err != nil {
		return nil, err
	}
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
	d, err = t.PropertiesUrl.MarshalBinary()
	if err != nil {
		return nil, err
	}
	buffer.Write(d)

	return buffer.Bytes(), nil
}

func (t *Token) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("error marshaling Token Issuance State %v", r)
		}
	}()

	txType, data := common.BytesUint64(data)

	if txType != uint64(types.TxTypeTokenCreate) {
		return fmt.Errorf("invalid transaction type, expecting token issuance")
	}

	err = t.URL.UnmarshalBinary(data)
	if err != nil {
		return err
	}

	i := t.URL.Size(nil)
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
	err = t.PropertiesUrl.UnmarshalBinary(data[i:])
	if err != nil {
		return err
	}

	return nil
}
