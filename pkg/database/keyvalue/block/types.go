// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"encoding"
	"fmt"
)

//go:generate go run gitlab.com/accumulatenetwork/core/schema/cmd/generate schema schema.yml -w schema_gen.go
//go:generate go run gitlab.com/accumulatenetwork/core/schema/cmd/generate types schema.yml -w types_gen.go
//go:generate go run github.com/rinchsan/gosimports/cmd/gosimports -w .

type entry interface {
	Type() entryType
	encoding.BinaryMarshaler
}

func (b *blockID) String() string {
	if b.Part == 0 {
		return fmt.Sprint(b.ID)
	}
	return fmt.Sprintf("%d{%d}", b.ID, b.Part)
}

func (b *blockID) Compare(c *blockID) int {
	if b.ID != c.ID {
		return int(b.ID - c.ID)
	}
	return int(b.Part - c.Part)
}
