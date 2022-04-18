package query

import "gitlab.com/accumulatenetwork/accumulate/types"

//go:generate go run ../../../tools/cmd/gen-types --package query types.yml
//go:generate go run ../../../tools/cmd/gen-enum --package query --out enums_gen.go enums.yml

func (*RequestKeyPageIndex) Type() types.QueryType { return types.QueryTypeKeyPageIndex }

// TxFetchMode specifies how much detail of the transactions should be included in the result set
type TxFetchMode uint64
