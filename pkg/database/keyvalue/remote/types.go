// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package remote

import "gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum  --package remote enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package remote types.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package remote --out unions_gen.go --language go-union types.yml

type callType int
type responseType int

type call interface {
	encoding.UnionValue
	Type() callType
}

type response interface {
	encoding.UnionValue
	Type() responseType
}
