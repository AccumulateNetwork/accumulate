// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package record

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package record -i TypeCode --rename TypeCode:typeCode ../../../tools/internal/typegen/enums.yml

// typeCode is identical to tools/internal/typegen.TypeCode, but we can't use
// that here because its internal, so we duplicate it.
type typeCode uint64
