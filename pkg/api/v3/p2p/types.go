// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

type eventType uint64

type event interface {
	encoding.UnionValue
	Type() eventType
}

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package p2p enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package p2p types.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --language go-union --package p2p --out unions_gen.go types.yml
