// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"sync/atomic"

	"golang.org/x/sync/errgroup"
)

type taskQueue struct {
	errg atomic.Pointer[errgroup.Group]
}

func newTaskQueue() *taskQueue {
	t := new(taskQueue)
	t.errg.Store(new(errgroup.Group))
	return t
}

func (t *taskQueue) Go(fn func() error) {
	t.errg.Load().Go(fn)
}

func (t *taskQueue) Flush() error {
	return t.errg.
		Swap(new(errgroup.Group)).
		Wait()
}
