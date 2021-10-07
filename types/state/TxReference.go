package state

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
)

//TxReference struct stores the transaction hash.  This state object will be a sub-state of
// the token state object so that it can be referenced via adi/account/{transaction#}
type TxReference struct {
	Chain
	TxId types.Bytes32 `json:"txid" form:"txid" query:"txid" validate:"required"`
}

func NewTxReference(url string, txid []byte) *TxReference {
	t := &TxReference{}
	t.TxId.FromBytes(txid)
	t.SetHeader(types.String(url), types.ChainTypeTransactionReference)
	return t
}

// GetType is an implemented interface that returns the chain type of the object
func (t *TxReference) GetType() uint64 {
	return t.Type.AsUint64()
}

// GetChainUrl returns the chain path for the object in the chain.
func (t *TxReference) GetChainUrl() string {
	return t.Chain.GetChainUrl()
}

func (t *TxReference) MarshalBinary() (data []byte, err error) {
	defer func() {
		if rerr := recover(); rerr != nil {
			err = fmt.Errorf("error marshaling transaction id reference state %v", rerr)
		}
	}()

	data, err = t.Chain.MarshalBinary()
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

	err = t.Chain.UnmarshalBinary(data)
	if err != nil {
		return fmt.Errorf("unable to unmarshal chain header, %v", err)
	}
	if t.Chain.Type != types.ChainTypeTransactionReference {
		return fmt.Errorf("expecting chain type transaction reference but received, %s", t.Chain.Type.Name())
	}
	if len(data) < 32 {
		return fmt.Errorf("unable to unmarshal txid reference state, length of data too short")
	}
	t.TxId.FromBytes(data[t.GetHeaderSize():])
	return nil
}
