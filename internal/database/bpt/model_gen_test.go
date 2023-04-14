// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

// GENERATED BY go run ./tools/cmd/gen-model. DO NOT EDIT.

//lint:file-ignore S1008,U1000 generated code

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type ChangeSet struct {
	logger logging.OptionalLogger
	store  record.Store

	bpt *BPT
}

func (c *ChangeSet) BPT() *BPT {
	return record.FieldGetOrCreate(&c.bpt, func() *BPT {
		return newBPT(c, c.logger.L, c.store, record.Key{}.Append("BPT"), "bpt", "bpt")
	})
}

func (c *ChangeSet) Resolve(key record.Key) (record.Record, record.Key, error) {
	if len(key) == 0 {
		return nil, nil, errors.InternalError.With("bad key for change set")
	}

	switch key[0] {
	case "BPT":
		return c.BPT(), key[1:], nil
	default:
		return nil, nil, errors.InternalError.With("bad key for change set")
	}
}

func (c *ChangeSet) IsDirty() bool {
	if c == nil {
		return false
	}

	if record.FieldIsDirty(c.bpt) {
		return true
	}

	return false
}

func (c *ChangeSet) WalkChanges(fn record.WalkFunc) error {
	if c == nil {
		return nil
	}

	var err error
	record.FieldWalkChanges(&err, c.bpt, fn)
	return err
}

func (c *ChangeSet) Commit() error {
	if c == nil {
		return nil
	}

	var err error
	record.FieldCommit(&err, c.bpt)

	return err
}
