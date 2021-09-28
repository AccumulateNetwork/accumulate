package api

import (
	"bytes"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/smt/common"
	"github.com/AccumulateNetwork/accumulated/types"
)

type MultiSigTx struct {
	TxHash types.Bytes32 `json:"hash" form:"url" query:"url" validate:"required"`
}

func (m *MultiSigTx) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	buf.Write(common.Int64Bytes(int64(types.TxTypeMultisigTx)))
	buf.Write(m.TxHash[:])

	return buf.Bytes(), nil
}

func (m *MultiSigTx) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("insufficent data to unmarshal MultiSigTx %v", err)
		}
	}()

	txType, data := common.BytesInt64(data)
	if txType != int64(types.TxTypeMultisigTx) {
		return fmt.Errorf("attempting to unmarshal incompatible type")
	}
	copy(m.TxHash[:], data)
	return nil
}
