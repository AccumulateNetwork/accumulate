package types

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
)

type TokenCirculationMode int

const (
	TokenCirculationMode_Burn TokenCirculationMode = iota
	TokenCirculationMode_Burn_and_Mint
)

type TokenIssuance struct {
	Type      string               `json:"type"`
	Supply    big.Int              `json:"supply"`
	Precision int8                 `json:"precision"`
	Symbol    string               `json:"symbol"`
	Mode      TokenCirculationMode `json:"mode"`
	Metadata  *json.RawMessage     `json:"metadata,omitempty"`
}

func NewTokenIssuance(symbol string, supply *big.Int, precision int8, mode TokenCirculationMode) (*TokenIssuance, error) {
	t := &TokenIssuance{}
	err := t.SetTokenParams(symbol, supply, precision, mode)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (t *TokenIssuance) SetTokenParams(symbol string, supply *big.Int, precision int8, mode TokenCirculationMode) error {
	if supply == nil {
		return fmt.Errorf("invalid supply")
	}

	maxsupply := big.NewInt(-1)
	if supply.Cmp(maxsupply) < 0 {
		return fmt.Errorf("invalid supply amount %s", supply.String())
	}

	if precision > 18 || precision < 0 {
		return fmt.Errorf("token precision must be between 0 and 18")
	}

	if len(symbol) > 10 {
		return fmt.Errorf("token ticker may not exceed 10 charaters in length")
	}

	t.Type = "AIM-0"
	t.Supply.Set(supply)
	t.Precision = precision
	t.Symbol = symbol
	t.Mode = mode

	return nil
}

func (t *TokenIssuance) SetMetadata(md *json.RawMessage) error {
	if md == nil {
		return fmt.Errorf("invalid metadata")
	}
	t.Metadata = &json.RawMessage{}
	copy((*t.Metadata)[:], (*md)[:])
	return nil
}

func (t *TokenIssuance) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.WriteByte(0)
	buffer.WriteByte(0)
	buffer.WriteByte(byte(len(t.Type)))
	buffer.WriteString(t.Type)
	buffer.WriteByte(byte(len(t.Symbol)))
	buffer.WriteString(t.Symbol)
	var sup Bytes32
	t.Supply.FillBytes(sup[:])
	buffer.Write(sup[:])
	buffer.WriteByte(byte(t.Precision))
	buffer.WriteByte(byte(t.Mode))

	mdlen := int16(0)
	if t.Metadata != nil {
		mdlen = int16(len(*t.Metadata))
	}
	buffer.WriteByte(byte(mdlen >> 8))
	buffer.WriteByte(byte(mdlen))
	if t.Metadata != nil {
		buffer.Write(*t.Metadata)
	}
	if len(buffer.Bytes()) > 0xFFFF {
		return nil, fmt.Errorf("error marshalling Token Isuance. Buffer exceeeds permitted size")
	}
	ret := buffer.Bytes()
	binary.BigEndian.PutUint16(ret[0:2], uint16(len(ret)-2))

	return ret, nil
}

func (t *TokenIssuance) UnmarshalBinary(data []byte) error {
	mlen := binary.BigEndian.Uint16(data)
	dlen := len(data)
	if mlen+2 != uint16(dlen) {
		return fmt.Errorf("cannot unmarshal Token Issuance object")
	}
	i := 2
	//copy the Type
	if dlen < i {
		return fmt.Errorf("cannot unmarshal Token Issuance object at Type length")
	}
	slen := int(data[i])
	i++
	if dlen < slen+i {
		return fmt.Errorf("cannot unmarshal Token Issuance object at Type")
	}
	t.Type = string(data[i : slen+i])
	i += slen

	//copy the symbol
	if dlen < i+1 {
		return fmt.Errorf("cannot unmarshal Token Issuance object at symbol length")
	}
	slen = int(data[i])
	i++
	if dlen < slen+i+1 {
		return fmt.Errorf("cannot unmarshal Token Issuance object at symbol")
	}
	t.Symbol = string(data[i : slen+i])
	i += slen

	if dlen < 32+i+1 {
		return fmt.Errorf("cannot unmarshal Token Issuance object at supply")
	}

	t.Supply.SetBytes(data[i : 32+i])
	i += 32

	if dlen < i+1 {
		return fmt.Errorf("cannot unmarshal Token Issuance object at precision")
	}
	t.Precision = int8(data[i])
	i++

	if dlen < i+1 {
		return fmt.Errorf("cannot unmarshal Token Issuance object at Circulation Mode")
	}
	t.Mode = TokenCirculationMode(data[i])
	i++

	if dlen < i+2 {
		return fmt.Errorf("cannot unmarshal Token Issuance object at Symbol length")
	}

	slen = int(data[i])<<8 + int(data[i+1])
	i += 2
	if slen > 0 {
		if dlen < slen+i {
			return fmt.Errorf("cannot unmarshal Token Issuance object at Metadata")
		}

		t.Metadata = &json.RawMessage{}
		*t.Metadata = append(*t.Metadata, data[i:slen+i]...)
	}

	return nil
}
