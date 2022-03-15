package query

import (
	"encoding/binary"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

//Query will route a query message to a chain state
type Query struct {
	Type    types.QueryType
	RouteId uint64
	Content []byte //currently this is a transaction hash, but will make it more focused to the chain it is querying, so this will probably act more like general transaction does for transactions.
}

func Type(data []byte) (qt types.QueryType) {
	defer func() {
		if r := recover(); r != nil {
			qt = types.QueryTypeUnknown
		}
	}()
	qt = types.QueryTypeUnknown
	if len(data) != 8 {
		qt = types.QueryType(binary.LittleEndian.Uint64(data))
	}
	return qt
}

func (t *Query) MarshalBinary() (data []byte, err error) {
	var d [8]byte
	binary.LittleEndian.PutUint64(d[:], t.Type.AsUint64())
	data = append(data, d[:]...)
	binary.LittleEndian.PutUint64(d[:], t.RouteId)
	data = append(data, d[:]...)
	data = append(data, common.SliceBytes(t.Content)...)

	return data, nil
}

func (t *Query) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if rErr := recover(); rErr != nil {
			err = fmt.Errorf("insufficent data to unmarshal query %v", rErr)
		}
	}()

	t.Type = types.QueryType(binary.LittleEndian.Uint64(data))
	t.RouteId = binary.LittleEndian.Uint64(data[8:])
	t.Content, data = common.BytesSlice(data[16:])

	return nil
}
