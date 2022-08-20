package query

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/encoding"
)

//go:generate go run ../../../../tools/cmd/gen-enum --package query --out enums_gen.go enums.yml
//go:generate go run ../../../../tools/cmd/gen-types --package query requests.yml responses.yml
//go:generate go run ../../../../tools/cmd/gen-types --package query --language go-union --out unions_gen.go requests.yml responses.yml

type QueryType uint64

// TxFetchMode specifies how much detail of the transactions should be included in the result set
type TxFetchMode uint64

// BlockFilterMode specifies which blocks should be excluded
type BlockFilterMode uint64

type Request interface {
	encoding.BinaryValue
	Type() QueryType
}
