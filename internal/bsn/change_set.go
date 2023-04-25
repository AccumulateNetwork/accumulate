// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

import (
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// New
func NewChangeSet(store storage.Beginner, logger log.Logger) *ChangeSet {
	c := new(ChangeSet)
	c.kvstore = store.Begin(true)
	c.store = record.KvStore{Store: c.kvstore}
	c.logger.Set(logger, "module", "database")
	return c
}

func (c *ChangeSet) Begin() *ChangeSet {
	d := new(ChangeSet)
	d.logger = c.logger
	d.store = c
	d.parent = c
	d.kvstore = c.kvstore.Begin(true)
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

// GetValue implements record.Store.
func (c *ChangeSet) GetValue(key *record.Key, value record.ValueWriter) error {
	v, err := resolveValue[record.ValueReader](c, key)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = value.LoadValue(v, false)
	return errors.UnknownError.Wrap(err)
}

// PutValue implements record.Store.
func (c *ChangeSet) PutValue(key *record.Key, value record.ValueReader) error {
	v, err := resolveValue[record.ValueWriter](c, key)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = v.LoadValue(value, true)
	return errors.UnknownError.Wrap(err)
}

func zero[T any]() T {
	var z T
	return z
}

// resolveValue resolves the value for the given key.
func resolveValue[T any](r record.Record, key *record.Key) (T, error) {
	var err error
	for key.Len() > 0 {
		r, key, err = r.Resolve(key)
		if err != nil {
			return zero[T](), errors.UnknownError.Wrap(err)
		}
	}

	if s, _, err := r.Resolve(nil); err == nil {
		r = s
	}

	v, ok := r.(T)
	if !ok {
		return zero[T](), errors.InternalError.WithFormat("bad key: %T is not value", r)
	}

	return v, nil
}
