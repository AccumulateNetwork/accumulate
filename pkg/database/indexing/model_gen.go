// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

// GENERATED BY go run ./tools/cmd/gen-model. DO NOT EDIT.

//lint:file-ignore S1008,U1000 generated code

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	record "gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type Log[V any] struct {
	logger    logging.OptionalLogger
	store     record.Store
	key       *record.Key
	blockSize uint64

	head  values.Value[*Block[V]]
	block map[logBlockMapKey]values.Value[*Block[V]]
}

func (c *Log[V]) Key() *record.Key { return c.key }

type logBlockKey struct {
	Level uint64
	Index uint64
}

type logBlockMapKey struct {
	Level uint64
	Index uint64
}

func (k logBlockKey) ForMap() logBlockMapKey {
	return logBlockMapKey{k.Level, k.Index}
}

func (c *Log[V]) getHead() values.Value[*Block[V]] {
	return values.GetOrCreate(c, &c.head, (*Log[V]).newHead)
}

func (c *Log[V]) newHead() values.Value[*Block[V]] {
	return values.NewValue(c.logger.L, c.store, c.key.Append("Head"), true, values.Struct[Block[V]]())
}

func (c *Log[V]) getBlock(level uint64, index uint64) values.Value[*Block[V]] {
	return values.GetOrCreateMap(c, &c.block, logBlockKey{level, index}, (*Log[V]).newBlock)
}

func (c *Log[V]) newBlock(k logBlockKey) values.Value[*Block[V]] {
	return values.NewValue(c.logger.L, c.store, c.key.Append("Block", k.Level, k.Index), false, values.Struct[Block[V]]())
}

func (c *Log[V]) Resolve(key *record.Key) (record.Record, *record.Key, error) {
	if key.Len() == 0 {
		return nil, nil, errors.InternalError.With("bad key for log (1)")
	}

	switch key.Get(0) {
	case "Head":
		return c.getHead(), key.SliceI(1), nil
	case "Block":
		if key.Len() < 3 {
			return nil, nil, errors.InternalError.With("bad key for log (2)")
		}
		level, okLevel := key.Get(1).(uint64)
		index, okIndex := key.Get(2).(uint64)
		if !okLevel || !okIndex {
			return nil, nil, errors.InternalError.With("bad key for log (3)")
		}
		v := c.getBlock(level, index)
		return v, key.SliceI(3), nil
	default:
		return nil, nil, errors.InternalError.With("bad key for log (4)")
	}
}

func (c *Log[V]) IsDirty() bool {
	if c == nil {
		return false
	}

	if values.IsDirty(c.head) {
		return true
	}
	for _, v := range c.block {
		if v.IsDirty() {
			return true
		}
	}

	return false
}

func (c *Log[V]) Walk(opts record.WalkOptions, fn record.WalkFunc) error {
	if c == nil {
		return nil
	}

	skip, err := values.WalkComposite(c, opts, fn)
	if skip || err != nil {
		return errors.UnknownError.Wrap(err)
	}
	values.WalkField(&err, c.head, c.newHead, opts, fn)
	values.WalkMap(&err, c.block, c.newBlock, nil, opts, fn)
	return err
}

func (c *Log[V]) Commit() error {
	if c == nil {
		return nil
	}

	var err error
	values.Commit(&err, c.head)
	for _, v := range c.block {
		values.Commit(&err, v)
	}

	return err
}
