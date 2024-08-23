// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"io"

	"gitlab.com/accumulatenetwork/core/schema/pkg/binary"
)

//go:generate go run gitlab.com/accumulatenetwork/core/schema/cmd/generate schema schema.yml -w schema_gen.go
//go:generate go run gitlab.com/accumulatenetwork/core/schema/cmd/generate types schema.yml -w types_gen.go
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-model --package indexing model.yml
//go:generate go run github.com/rinchsan/gosimports/cmd/gosimports -w .

func (b *Block[V]) CopyAsInterface() any { return b.Copy() }

func (b *Block[V]) UnmarshalBinaryFrom(r io.Reader) error {
	return b.UnmarshalBinaryV2(binary.NewDecoder(r))
}
