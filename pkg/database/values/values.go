// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package values

import (
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
)

// Value records a value.
type Value[T any] interface {
	database.Record
	Get() (T, error)
	GetAs(any) error
	Put(T) error
}

// List records an unordered list of values as a single record.
type List[T any] interface {
	Value[[]T]
	Add(...T) error
}

// Set records an ordered list of values as a single record.
type Set[T any] interface {
	List[T]
	Remove(T) error
	Index(T) (int, error)
	Find(T) (T, error)
}

// Counted records an insertion-ordered list of values as separate records plus
// a record for the count.
type Counted[T any] interface {
	database.Record
	Get(int) (T, error)
	Put(T) error
	Count() (int, error)
	GetAll() ([]T, error)
	Last() (int, T, error)
	Overwrite([]T) error
}

// NewValue returns a new value using the given encodable value.
func NewValue[T any](logger log.Logger, store database.Store, key *database.Key, name string, allowMissing bool, ev encodableValue[T]) Value[T] {
	return newValue(logger, store, key, name, allowMissing, ev)
}

// NewList returns a new list using the given encoder and comparison.
func NewList[T any](logger log.Logger, store database.Store, key *database.Key, namefmt string, encoder encodableValue[T]) List[T] {
	return newList(logger, store, key, namefmt, encoder)
}

// NewSet returns a new set using the given encoder and comparison.
func NewSet[T any](logger log.Logger, store database.Store, key *database.Key, namefmt string, encoder encodableValue[T], cmp func(u, v T) int) Set[T] {
	return newSet(logger, store, key, namefmt, encoder, cmp)
}

// NewCounted returns a new counted using the given encodable value type.
func NewCounted[T any](logger log.Logger, store database.Store, key *database.Key, namefmt string, new func() encodableValue[T]) Counted[T] {
	return newCounted(logger, store, key, namefmt, new)
}