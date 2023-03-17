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
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type PartitionBatch struct {
	id string
	*database.Batch
}

func (c *ChangeSet) Partition(id string) *PartitionBatch {
	id = strings.ToLower(id)
	if b, ok := c.partition[partitionKey{id}]; ok {
		return b
	}

	if c.partition == nil {
		c.partition = map[partitionKey]*PartitionBatch{}
	}

	var b *database.Batch
	if c.parent == nil {
		// TODO It would be better to do c.store.(record.KvStore).WithPrefix(id)
		s := c.kvstore.WithPrefix(id).Begin(true)
		b = database.NewBatch(id, s, true, c.logger)
		b.SetObserver(execute.NewDatabaseObserver())
	} else {
		b = c.parent.Partition(id).Begin(true)
	}

	pb := &PartitionBatch{id, b}
	c.partition[partitionKey{id}] = pb
	return pb
}

func (b *PartitionBatch) Resolve(key record.Key) (record.Record, record.Key, error) {
	if len(key) < 2 {
		return nil, nil, errors.InternalError.With("bad key for partition batch")
	}

	k1, ok1 := key[0].(string)
	k2, ok2 := key[1].(string)
	if !ok1 || !ok2 || k1 != "Partition" || k2 != b.id {
		return nil, nil, errors.InternalError.With("bad key for partition batch")
	}
	return b.Batch.Resolve(key[2:])
}
