// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import "gitlab.com/accumulatenetwork/accumulate/pkg/types/record"

type Query[V any] interface {
	// Before returns the target entry if the match was exact, the entry before
	// the target if one exists, or an error result.
	Before() QueryResult[V]

	// Exact returns the target entry if the match was exact, or an error
	// result.
	Exact() QueryResult[V]

	// After returns the target entry if the match was exact, the entry after
	// the target if one exists, or an error result.
	After() QueryResult[V]
}

type QueryResult[V any] interface {
	// Get returns the value of the target entry or an error if the search failed.
	Get() (*record.Key, V, error)

	// TODO Add Next() (ChainSearchResult2, bool) that returns the next entry in
	// the index.
}

type ValueResult[V any] struct {
	Key   *record.Key
	Value *Value[V]
}

func (r ValueResult[V]) Before() QueryResult[V] { return r }
func (r ValueResult[V]) Exact() QueryResult[V]  { return r }
func (r ValueResult[V]) After() QueryResult[V]  { return r }

func (r ValueResult[V]) Get() (*record.Key, V, error) {
	v, err := r.Value.Get()
	return r.Key, v, err
}

type ErrorResult[V any] struct{ error }

func (r ErrorResult[V]) Unwrap() error                { return r.error }
func (r ErrorResult[V]) Before() QueryResult[V]       { return r }
func (r ErrorResult[V]) Exact() QueryResult[V]        { return r }
func (r ErrorResult[V]) After() QueryResult[V]        { return r }
func (r ErrorResult[V]) Get() (*record.Key, V, error) { var z V; return nil, z, r.error }
