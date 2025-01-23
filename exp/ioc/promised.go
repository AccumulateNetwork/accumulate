// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package ioc

import (
	"gitlab.com/accumulatenetwork/accumulate/exp/promise"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type Promised interface {
	Resolve(any) error
}

type PromisedOf[T any] struct {
	promise promise.Promise[T]
	resolve func(T)
}

func NewPromisedOf[T any]() *PromisedOf[T] {
	p, r, _ := promise.New[T]()
	return &PromisedOf[T]{p, r}
}

func (r *PromisedOf[T]) Resolve(v any) error {
	u, ok := v.(T)
	if !ok {
		var z T
		return errors.Conflict.With("want %T, got %T", z, v)
	}
	r.resolve(u)
	return nil
}

func (r *PromisedOf[T]) Get() T {
	v, err := r.promise.Result().Get()
	if err != nil {
		panic(err)
	}
	return v
}
