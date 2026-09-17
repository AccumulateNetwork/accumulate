// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import "io"

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package database types.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-model --package database model.yml
//go:generate go run gitlab.com/accumulatenetwork/core/schema/cmd/generate schema schema.yml -w schema_gen.go
//go:generate go run gitlab.com/accumulatenetwork/core/schema/cmd/generate types schema.yml -w types2_gen.go
//go:generate go run github.com/rinchsan/gosimports/cmd/gosimports -w .

// UnmarshalBinaryFrom lets a BlockLedger be held by a values.Value.
func (v *BlockLedger) UnmarshalBinaryFrom(rd io.Reader) error {
	b, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return v.UnmarshalBinary(b)
}

// CopyAsInterface lets a BlockLedger be held by a values.Value.
func (v *BlockLedger) CopyAsInterface() interface{} { return v.Copy() }
