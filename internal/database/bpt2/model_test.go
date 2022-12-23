// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt2_test

import (
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/bpt2"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type BPT = bpt2.BPT

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-model --package bpt2_test --out model_gen_test.go model_test.yml

func newBPT(_ *ChangeSet, logger log.Logger, store record.Store, key record.Key, _, label string) *BPT {
	return bpt2.New(logger, store, key, 3, label)
}

func (cs *ChangeSet) Begin(writable bool) *ChangeSet {
	d := new(ChangeSet)
	d.logger = cs.logger
	d.store = cs
	return d
}

// GetValue implements record.Store.
func (b *ChangeSet) GetValue(key record.Key, value record.ValueWriter) error {
	v, err := resolveValue[record.ValueReader](b, key)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = value.LoadValue(v, false)
	return errors.UnknownError.Wrap(err)
}

// PutValue implements record.Store.
func (b *ChangeSet) PutValue(key record.Key, value record.ValueReader) error {
	v, err := resolveValue[record.ValueWriter](b, key)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = v.LoadValue(value, true)
	return errors.UnknownError.Wrap(err)
}

// resolveValue resolves the value for the given key.
func resolveValue[T any](c *ChangeSet, key record.Key) (T, error) {
	var r record.Record = c
	var err error
	for len(key) > 0 {
		r, key, err = r.Resolve(key)
		if err != nil {
			var z T
			return z, errors.UnknownError.Wrap(err)
		}
	}

	if s, _, err := r.Resolve(nil); err == nil {
		r = s
	}

	v, ok := r.(T)
	if !ok {
		var z T
		return z, errors.InternalError.WithFormat("bad key: %T is not value", r)
	}

	return v, nil
}
