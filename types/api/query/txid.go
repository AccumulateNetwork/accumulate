package query

import (
	"bytes"
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
