// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package snapshot enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package snapshot types.yml

type SectionType uint64
