package api

import (
	"encoding"

	"github.com/AccumulateNetwork/accumulated/types"
)

type queryRequest interface {
	encoding.BinaryMarshaler
	Type() types.QueryType
}

func NewQueryDirect(c ABCIQueryClient) Querier {
	return queryDirect{c}
}

func NewQueryDispatch(c []ABCIQueryClient) Querier {
	return queryDispatch{c}
}
