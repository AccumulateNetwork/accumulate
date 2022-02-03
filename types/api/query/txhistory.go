package query

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/types"
)

type RequestTxHistory struct {
	ChainId types.Bytes32
	Start   int64
	Limit   int64
}

func (*RequestTxHistory) Type() types.QueryType { return types.QueryTypeTxHistory }

func (t *RequestTxHistory) MarshalBinary() (data []byte, err error) {
	var buff bytes.Buffer
	var d [8]byte
	binary.LittleEndian.PutUint64(d[:], uint64(t.Start))
	buff.Write(d[:])

	binary.LittleEndian.PutUint64(d[:], uint64(t.Limit))
	buff.Write(d[:])

	buff.Write(t.ChainId[:])

	return buff.Bytes(), nil
}

func (t *RequestTxHistory) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("error unmarshaling RequestTxHistory %v", r)
		}
	}()

	t.Start = int64(binary.LittleEndian.Uint64(data[:]))
	t.Limit = int64(binary.LittleEndian.Uint64(data[8:]))
	t.ChainId.FromBytes(data[16:])
	return nil
}
