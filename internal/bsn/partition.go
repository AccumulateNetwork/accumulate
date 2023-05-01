// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

import (
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func (c *ChangeSet) Partition(id string) *database.Batch {
	id = strings.ToLower(id)
	if b, ok := c.partition[partitionMapKey{id}]; ok {
		return b
	}

	if c.partition == nil {
		c.partition = map[partitionMapKey]*database.Batch{}
	}

	var b *database.Batch
	if c.parent == nil {
		s := c.kvstore.Begin(record.NewKey(id+"·"), true)
		b = database.NewBatch(id, s, true, c.logger)
		b.SetObserver(execute.NewDatabaseObserver())
	} else {
		b = c.parent.Partition(id).Begin(true)
	}

	c.partition[partitionMapKey{id}] = b
	return b
}
