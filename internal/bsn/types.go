// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package bsn --out enums_gen.go -i TypeCode ../../tools/internal/typegen/enums.yml

type TypeCode uint64
