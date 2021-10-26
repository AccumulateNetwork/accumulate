package query

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/smt/common"
	"github.com/AccumulateNetwork/accumulated/types"
)

type ResponseByTxId struct {
	TxState        types.Bytes
	TxPendingState types.Bytes
	TxSynthTxIds   types.Bytes
}

type RequestByTxId struct {
	TxId types.Bytes32
}

func (t *ResponseByTxId) Size() int {
	var d [8]byte
	l1 := len(t.TxState)
	l2 := len(t.TxPendingState)
	l3 := len(t.TxSynthTxIds)

	n1 := binary.PutUvarint(d[:], uint64(l1))
	n2 := binary.PutUvarint(d[:], uint64(l2))
	n3 := binary.PutUvarint(d[:], uint64(l3))
	return l1 + l2 + l3 + n1 + n2 + n3
}

func (t *ResponseByTxId) MarshalBinary() ([]byte, error) {
	var buff bytes.Buffer
	buff.Write(common.SliceBytes(t.TxState))
	buff.Write(common.SliceBytes(t.TxPendingState))
	buff.Write(common.SliceBytes(t.TxSynthTxIds))

	return buff.Bytes(), nil
}

func (t *ResponseByTxId) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("error unmarshaling ResponseByTxId data %v", r)
		}
	}()

	t.TxState, data = common.BytesSlice(data)
	t.TxPendingState, data = common.BytesSlice(data)
	t.TxSynthTxIds, _ = common.BytesSlice(data)

	return nil
}

func (t *RequestByTxId) MarshalBinary() (data []byte, err error) {
	return t.TxId[:], nil
}

func (t *RequestByTxId) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("error unmarshaling RequestByTxId data %v", r)
		}
	}()
	if len(data) < 32 {
		return fmt.Errorf("insufficient data for txid in RequestByTxId")
	}
	t.TxId.FromBytes(data)
	return nil
}
