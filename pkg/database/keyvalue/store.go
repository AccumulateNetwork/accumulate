// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package keyvalue

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// Store is a key-value store.
type Store interface {
	// Get loads a value.
	Get(*record.Key) ([]byte, error)

	// Put stores a value.
	Put(*record.Key, []byte) error

	// Delete deletes a key-value pair.
	Delete(*record.Key) error

	// ForEach iterates over each value.
	ForEach(func(*record.Key, []byte) error) error
}

// RecordStore implements [database.Store].
type RecordStore struct {
	Store Store
}

var _ database.Store = RecordStore{}

// Unwrap returns the underlying store.
func (s RecordStore) Unwrap() Store { return s.Store }

// GetValue loads the raw value and calls value.LoadBytes.
func (s RecordStore) GetValue(key *record.Key, value database.Value) error {
	b, err := s.Store.Get(key)
	if err != nil {
		return err // Do not wrap - allow NotFoundError to propagate
	}

	// Tests may 'delete' an account by setting its value to nil
	if len(b) == 0 {
		return (*database.NotFoundError)(key)
	}

	err = value.LoadBytes(b, false)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	return nil
}

// PutValue marshals the value and stores it.
func (s RecordStore) PutValue(key *record.Key, value database.Value) error {
	// TODO Detect conflicting writes at this level, when multiple batches are
	// created from the same database
	v, _, err := value.GetValue()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	b, err := v.MarshalBinary()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = s.Store.Put(key, b)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	return nil
}
