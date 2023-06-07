// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package services

import "gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package services enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package services types.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package services --language go-union --out unions_gen.go types.yml

type ServiceType int
type KeyStorageType int

type Service interface {
	encoding.UnionValue
	Type() ServiceType
	IsValid() error
}

type KeyStorage interface {
	encoding.UnionValue
	Type() KeyStorageType
}
