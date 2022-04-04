package query

import (
	"gitlab.com/accumulatenetwork/accumulate/types"
)

func (*RequestDataEntry) Type() types.QueryType { return types.QueryTypeData }

func (*RequestDataEntrySet) Type() types.QueryType { return types.QueryTypeDataSet }
