package api

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulated/smt/common"
)

//Query will route a query message to a chain state
type Query struct {
	Url     string
	RouteId uint64
	ChainId []byte
	Content []byte //currently this is a transaction hash, but will make it more focused to the chain it is querying, so this will probably act more like general transaction does for transactions.
}

func (t *Query) MarshalBinary() (data []byte, err error) {
	data = append(data, common.SliceBytes([]byte(t.Url))...)
	data = append(data, common.Uint64Bytes(t.RouteId)...)
	data = append(data, common.SliceBytes(t.ChainId)...)
	data = append(data, common.SliceBytes(t.Content)...)

	return data, nil
}

func (t *Query) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if rErr := recover(); rErr != nil {
			err = fmt.Errorf("insufficent data to unmarshal query %v", rErr)
		}
	}()

	url, data := common.BytesSlice(data)
	t.Url = string(url)
	t.RouteId, data = common.BytesUint64(data)
	t.ChainId, data = common.BytesSlice(data)
	t.Content, data = common.BytesSlice(data)

	return nil
}
