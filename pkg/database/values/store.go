// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package values

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// RecordStore implements [database.Store] using a [database.Record].
type RecordStore struct {
	Record database.Record
}

func (s RecordStore) Unwrap() database.Record { return s.Record }

// GetValue implements record.Store.
func (s RecordStore) GetValue(key *database.Key, dst database.Value) error {
	src, err := Resolve[database.Value](s.Record, key)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = dst.LoadValue(src, false)
	return errors.UnknownError.Wrap(err)
}

// PutValue implements database.Store.
func (s RecordStore) PutValue(key *database.Key, src database.Value) error {
	dst, err := Resolve[database.Value](s.Record, key)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = dst.LoadValue(src, true)
	return errors.UnknownError.Wrap(err)
}

// Resolve resolves the value for the given key.
func Resolve[T any](r database.Record, key *database.Key) (T, error) {
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
