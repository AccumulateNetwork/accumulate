package api

import (
	"time"
)

//go:generate go run ../../../tools/cmd/gentypes --package api types.yml
//go:generate go run ../../../tools/cmd/genapi --package api methods.yml
//go:generate go run github.com/golang/mock/mockgen -source types.go -destination ../../mock/api/types.go

type Querier interface {
	QueryUrl(url string) (*QueryResponse, error)
	QueryDirectory(url string, pagination QueryPagination, opts QueryOptions) (*QueryResponse, error)
	QueryChain(id []byte) (*QueryResponse, error)
	QueryTx(id []byte, wait time.Duration) (*QueryResponse, error)
	QueryTxHistory(url string, start, count uint64) (*QueryMultiResponse, error)
	QueryData(url string, entryHash [32]byte) (*QueryResponse, error)
	QueryDataSet(url string, pagination QueryPagination, opts QueryOptions) (*QueryResponse, error)
	QueryKeyPageIndex(url string, key []byte) (*QueryResponse, error)
}
