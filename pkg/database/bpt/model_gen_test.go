// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

// GENERATED BY go run ./tools/cmd/gen-model. DO NOT EDIT.

//lint:file-ignore S1008,U1000 generated code

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	record "gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type ChangeSet struct {
	logger logging.OptionalLogger
	store  record.Store

	bpt BPT
}

func (c *ChangeSet) Key() *record.Key { return nil }

func (c *ChangeSet) BPT() BPT {
	return values.GetOrCreate(&c.bpt, (*ChangeSet).newBPT, c)
}

func (c *ChangeSet) newBPT() BPT {
	return newBPT(c, c.logger.L, c.store, (*record.Key)(nil).Append("BPT"), "bpt", "bpt")
}

func (c *ChangeSet) Resolve(key *record.Key) (record.Record, *record.Key, error) {
	if key.Len() == 0 {
		return nil, nil, errors.InternalError.With("bad key for change set")
	}

	switch key.Get(0) {
	case "BPT":
		return c.BPT(), key.SliceI(1), nil
	default:
		return nil, nil, errors.InternalError.With("bad key for change set")
	}
}

func (c *ChangeSet) IsDirty() bool {
	if c == nil {
		return false
	}

	if values.IsDirty(c.bpt) {
		return true
	}

	return false
}

func (c *ChangeSet) Walk(opts record.WalkOptions, fn record.WalkFunc) error {
	if c == nil {
		return nil
	}

	skip, err := values.WalkComposite(c, opts, fn)
	if skip || err != nil {
		return errors.UnknownError.Wrap(err)
	}
	values.WalkField(&err, c.bpt, c.BPT, opts, fn)
	return err
}

func (c *ChangeSet) Commit() error {
	if c == nil {
		return nil
	}

	var err error
	values.Commit(&err, c.bpt)

	return err
}
