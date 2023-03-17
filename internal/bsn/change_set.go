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

func NewChangeSet(store storage.KeyValueStore, logger log.Logger) *ChangeSet {
	c := new(ChangeSet)
	c.kvstore = store
	c.store = record.KvStore{Store: store.Begin(true)}
	c.logger.Set(logger, "module", "database")
	return c
}

func (c *ChangeSet) Begin() *ChangeSet {
	d := new(ChangeSet)
	d.logger = c.logger
	d.store = c
	d.parent = c
	return d
}

func (c *ChangeSet) Commit() error {
	err := c.baseCommit()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	kvs, ok := c.store.(record.KvStore)
	if !ok {
		return nil
	}

	err = kvs.Store.Commit()
	return errors.UnknownError.Wrap(err)
}

func (c *ChangeSet) Discard() {
	if kvs, ok := c.store.(record.KvStore); ok {
		kvs.Store.Discard()
	}
}

// GetValue implements record.Store.
func (c *ChangeSet) GetValue(key record.Key, value record.ValueWriter) error {
	v, err := resolveValue[record.ValueReader](c, key)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = value.LoadValue(v, false)
	return errors.UnknownError.Wrap(err)
}

// PutValue implements record.Store.
func (c *ChangeSet) PutValue(key record.Key, value record.ValueReader) error {
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
func resolveValue[T any](c *ChangeSet, key record.Key) (T, error) {
	var r record.Record = c
	var err error
	for len(key) > 0 {
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
