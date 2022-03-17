package query

import (
	"gitlab.com/accumulatenetwork/accumulate/types"
)

func (*RequestTxHistory) Type() types.QueryType { return types.QueryTypeTxHistory }
