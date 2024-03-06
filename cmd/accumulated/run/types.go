// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum  --package run enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package run config.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package run --language go-union --out unions_gen.go config.yml

type (
	PrivateKeyType int
)
