// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package merkle types.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package merkle enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-model --package merkle model.yml

// ChainType is the type of a chain belonging to an account.
type ChainType uint64
