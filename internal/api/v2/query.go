package api

import (
	"encoding"
	"github.com/AccumulateNetwork/accumulate/networks/connections"
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

func NewQueryDirect(connRoute connections.Route, opts QuerierOptions) Querier {
	return &queryDirect{opts, connRoute}
}

func NewQueryDispatch(connRtr connections.ConnectionRouter, opts QuerierOptions) Querier {
	return &queryDispatch{opts, connRtr}
}
