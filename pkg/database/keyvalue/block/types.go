// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import "gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"

type entryType int

type entry interface {
	encoding.UnionValue
	Type() entryType
}

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package block enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package block types.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package block --language go-union --out unions_gen.go types.yml
