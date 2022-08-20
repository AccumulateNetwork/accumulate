package record

import (
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// List records an unordered list of values as a single record.
type List[T any] struct {
	Value[[]T]
}

// NewList returns a new List using the given encoder and comparison.
func NewList[T any](logger log.Logger, store Store, key Key, namefmt string, encoder encodableValue[T]) *List[T] {
	s := &List[T]{}
	s.Value = *NewValue[[]T](logger, store, key, namefmt, true, &sliceValue[T]{encoder: encoder})
	return s
}

// Add inserts values into the set, sorted.
func (s *List[T]) Add(v ...T) error {
	l, err := s.Get()
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	err = s.Value.Put(append(l, v...))
	return errors.Wrap(errors.StatusUnknownError, err)
}

// IsDirty implements Record.IsDirty.
func (s *List[T]) IsDirty() bool {
	if s == nil {
		return false
	}
	return s.Value.IsDirty()
}

// Commit implements Record.Commit.
func (s *List[T]) Commit() error {
	if s == nil {
		return nil
	}
	err := s.Value.Commit()
	return errors.Wrap(errors.StatusUnknownError, err)
}
