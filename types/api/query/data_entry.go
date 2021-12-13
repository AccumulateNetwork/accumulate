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
