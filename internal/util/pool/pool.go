// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pool

import "sync"

func New[T any]() *Pool[*T] {
	return (*Pool[*T])(&sync.Pool{New: func() any { return new(T) }})
}

type Pool[T any] sync.Pool

func (p *Pool[T]) Get() T {
	return (*sync.Pool)(p).Get().(T)
}

func (p *Pool[T]) Put(v T) {
	(*sync.Pool)(p).Put(v)
}
