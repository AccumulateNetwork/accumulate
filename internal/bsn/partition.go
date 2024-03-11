// Copyright 2024 The Accumulate Authors
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

	b := c.newPartition(partitionKey{id})
	c.partition[partitionMapKey{id}] = b
	return b
}

func (c *ChangeSet) newPartition(key partitionKey) *database.Batch {
	if c.parent != nil {
		return c.parent.Partition(key.ID).Begin(true)
	}

	s := c.kvstore.Begin(record.NewKey(key.ID+"·"), true)
	b := database.NewBatch(key.ID, s, true, c.logger)
	b.SetObserver(execute.NewDatabaseObserver())
	return b
}
