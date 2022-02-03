package query

import (
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

type RequestDataEntry struct {
	protocol.RequestDataEntry
}

type RequestDataEntrySet struct {
	protocol.RequestDataEntrySet
}

func (*RequestDataEntry) Type() types.QueryType { return types.QueryTypeData }

func (*RequestDataEntrySet) Type() types.QueryType { return types.QueryTypeDataSet }
