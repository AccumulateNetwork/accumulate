// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

import (
	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// New
func NewChangeSet(store keyvalue.Beginner, logger log.Logger) *ChangeSet {
	c := new(ChangeSet)
	c.kvstore = store.Begin(nil, true)
	c.store = keyvalue.RecordStore{Store: c.kvstore}
	c.logger.Set(logger, "module", "database")
	return c
}

func (c *ChangeSet) Begin() *ChangeSet {
	d := new(ChangeSet)
	d.logger = c.logger
	d.store = values.RecordStore{Record: c}
	d.parent = c
	d.kvstore = c.kvstore.Begin(nil, true)
	return d
}

func (c *ChangeSet) Commit() error {
	err := c.baseCommit()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = c.kvstore.Commit()
	return errors.UnknownError.Wrap(err)
}

func (c *ChangeSet) Discard() {
	for _, b := range c.partition {
		b.Discard()
	}
	c.kvstore.Discard()
}
