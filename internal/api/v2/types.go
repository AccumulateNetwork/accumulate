package api

import (
	"time"
)

//go:generate go run ../../../tools/cmd/gentypes --package api types.yml
//go:generate go run ../../../tools/cmd/genapi --package api methods.yml
//go:generate go run github.com/golang/mock/mockgen -source types.go -destination ../../mock/api/types.go

type Querier interface {
	QueryUrl(url string) (interface{}, error)
	QueryDirectory(url string, pagination QueryPagination, opts QueryOptions) (*MultiResponse, error)
	QueryChain(id []byte) (*ChainQueryResponse, error)
	QueryTx(id []byte, wait time.Duration) (*TransactionQueryResponse, error)
	QueryTxHistory(url string, pagination QueryPagination) (*MultiResponse, error)
	QueryData(url string, entryHash [32]byte) (*ChainQueryResponse, error)
	QueryDataSet(url string, pagination QueryPagination, opts QueryOptions) (*MultiResponse, error)
	QueryKeyPageIndex(url string, key []byte) (*ChainQueryResponse, error)
}
