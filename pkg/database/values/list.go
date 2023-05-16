// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package values

import (
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type list[T any] struct {
	value[[]T]
}

func newList[T any](logger log.Logger, store database.Store, key *database.Key, namefmt string, encoder encodableValue[T]) *list[T] {
	s := &list[T]{}
	s.value = *newValue[[]T](logger, store, key, namefmt, true, &sliceValue[T]{encoder: encoder})
	return s
}

// Add inserts values into the set, sorted.
func (s *list[T]) Add(v ...T) error {
	l, err := s.Get()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = s.value.Put(append(l, v...))
	return errors.UnknownError.Wrap(err)
}

// IsDirty implements Record.IsDirty.
func (s *list[T]) IsDirty() bool {
	if s == nil {
		return false
	}
	return s.value.IsDirty()
}

// Commit implements Record.Commit.
func (s *list[T]) Commit() error {
	if s == nil {
		return nil
	}
	err := s.value.Commit()
	return errors.UnknownError.Wrap(err)
}

func (v *list[T]) Walk(opts database.WalkOptions, fn database.WalkFunc) error {
	if opts.Modified && !v.IsDirty() {
		return nil
	}

	// If the set is empty, skip it
	u, err := v.Get()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if len(u) == 0 {
		return nil
	}

	// Walk the record
	_, err = fn(v)
	return err
}
