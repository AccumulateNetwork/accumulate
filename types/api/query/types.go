package query

import "gitlab.com/accumulatenetwork/accumulate/types"

//go:generate go run ../../../tools/cmd/gentypes --package query types.yml

func (*RequestKeyPageIndex) Type() types.QueryType { return types.QueryTypeKeyPageIndex }
