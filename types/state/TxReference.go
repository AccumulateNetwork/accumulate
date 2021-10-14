package state

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"
)

//TxReference struct stores the transaction hash.  This state object will be a sub-state of
// the token state object so that it can be referenced via adi/account/{transaction#}
type TxReference struct {
	ChainHeader
	TxId types.Bytes32 `json:"txid" form:"txid" query:"txid" validate:"required"`
}

func NewTxReference(url string, txid []byte) *TxReference {
	t := &TxReference{}
	t.TxId.FromBytes(txid)
	t.SetHeader(types.String(url), types.ChainTypeTransactionReference)
	return t
}

func (t *TxReference) MarshalBinary() (data []byte, err error) {
	defer func() {
		if rerr := recover(); rerr != nil {
			err = fmt.Errorf("error marshaling transaction id reference state %v", rerr)
		}
	}()

	data, err = t.ChainHeader.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("unable to marshal chain header, %v", err)
	}

	data = append(data, t.TxId.Bytes()...)
	return data, nil
}

func (t *TxReference) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if rerr := recover(); rerr != nil {
			err = fmt.Errorf("error unmarshaling transaction id ref state %v", rerr)
		}
	}()

	err = t.ChainHeader.UnmarshalBinary(data)
	if err != nil {
		return fmt.Errorf("unable to unmarshal chain header, %v", err)
	}
	if t.ChainHeader.Type != types.ChainTypeTransactionReference {
		return fmt.Errorf("expecting chain type transaction reference but received, %s", t.ChainHeader.Type.Name())
	}
	if len(data) < 32 {
		return fmt.Errorf("unable to unmarshal txid reference state, length of data too short")
	}
	t.TxId.FromBytes(data[t.GetHeaderSize():])
	return nil
}
