package api

import (
	"encoding"
	"time"

	"github.com/AccumulateNetwork/accumulate/types"
)

type queryRequest interface {
	encoding.BinaryMarshaler
	Type() types.QueryType
}

type QuerierOptions struct {
	TxMaxWaitTime time.Duration
}

func NewQueryDirect(c ABCIQueryClient, opts QuerierOptions) Querier {
	return &queryDirect{opts, c}
}

func NewQueryDispatch(c []ABCIQueryClient, opts QuerierOptions) Querier {
	return &queryDispatch{opts, c}
}
