package api

import (
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

type Querier interface {
	QueryUrl(url *url.URL, opts QueryOptions) (interface{}, error)
	QueryDirectory(url *url.URL, pagination QueryPagination, opts QueryOptions) (*MultiResponse, error)
	QueryChain(id []byte) (*ChainQueryResponse, error)
	QueryTx(id []byte, wait time.Duration, opts QueryOptions) (*TransactionQueryResponse, error)
	QueryTxHistory(url *url.URL, pagination QueryPagination) (*MultiResponse, error)
	QueryData(url *url.URL, entryHash [32]byte) (*ChainQueryResponse, error)
	QueryDataSet(url *url.URL, pagination QueryPagination, opts QueryOptions) (*MultiResponse, error)
	QueryKeyPageIndex(url *url.URL, key []byte) (*ChainQueryResponse, error)
}

func NewQueryDirect(subnet string, opts Options) Querier {
	return &queryDirect{opts, subnet}
}

func NewQueryDispatch(opts Options) Querier {
	return &queryDispatch{opts}
}
