package api

import (
	"bytes"
	"encoding"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/proto"
)

type MultiSigTx struct {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
	TxHash types.Bytes32 `json:"hash" form:"url" query:"url" validate:"required"`
}

func (m *MultiSigTx) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte(byte(proto.AccInstruction_Multisig_Transaction))
	buf.Write(m.TxHash[:])

	return buf.Bytes(), nil
}

func (m *MultiSigTx) UnmarshalBinary(data []byte) error {
	if len(data) != 33 {
		return fmt.Errorf("insufficent data to unmarshal MultiSigTx")
	}
	if data[0] != byte(proto.AccInstruction_Multisig_Transaction) {
		return fmt.Errorf("attempting to unmarshal incompatible type")
	}
	copy(m.TxHash[:], data[1:])
	return nil
}
