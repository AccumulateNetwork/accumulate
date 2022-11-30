// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package query

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package query --out enums_gen.go enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package query requests.yml responses.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package query --language go-union --out unions_gen.go requests.yml responses.yml

type QueryType uint64

// TxFetchMode specifies how much detail of the transactions should be included in the result set
type TxFetchMode uint64

// BlockFilterMode specifies which blocks should be excluded
type BlockFilterMode uint64

type Request interface {
	encoding.UnionValue
	Type() QueryType
}
