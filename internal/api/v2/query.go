package api

import (
	"time"
)

type Querier interface {
	QueryUrl(url string, opts QueryOptions) (interface{}, error)
	QueryDirectory(url string, pagination QueryPagination, opts QueryOptions) (*MultiResponse, error)
	QueryChain(id []byte) (*ChainQueryResponse, error)
	QueryTx(id []byte, wait time.Duration, opts QueryOptions) (*TransactionQueryResponse, error)
	QueryTxHistory(url string, pagination QueryPagination) (*MultiResponse, error)
	QueryData(url string, entryHash [32]byte) (*ChainQueryResponse, error)
	QueryDataSet(url string, pagination QueryPagination, opts QueryOptions) (*MultiResponse, error)
	QueryKeyPageIndex(url string, key []byte) (*ChainQueryResponse, error)
}

func NewQueryDirect(subnet string, opts Options) Querier {
	return &queryDirect{opts, subnet}
}

func NewQueryDispatch(opts Options) Querier {
	return &queryDispatch{opts}
}
