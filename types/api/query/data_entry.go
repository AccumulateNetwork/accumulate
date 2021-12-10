package query

import (
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
)

type RequestDataEntry struct {
	protocol.RequestDataEntry
}

type RequestDataEntrySet struct {
	protocol.RequestDataEntrySet
}

func (*RequestDataEntry) Type() types.QueryType { return types.QueryTypeData }

func (*RequestDataEntrySet) Type() types.QueryType { return types.QueryTypeDataSet }

func (v *RequestDataEntry) MarshalBinary() ([]byte, error) {
	return v.RequestDataEntry.MarshalBinary()
}

func (v *RequestDataEntrySet) MarshalBinary() ([]byte, error) {
	return v.RequestDataEntrySet.MarshalBinary()
}

func (v *RequestDataEntry) UnmarshalBinary(data []byte) error {
	return v.RequestDataEntry.UnmarshalBinary(data)
}

func (v *RequestDataEntrySet) UnmarshalBinary(data []byte) error {
	return v.RequestDataEntrySet.UnmarshalBinary(data)
}
