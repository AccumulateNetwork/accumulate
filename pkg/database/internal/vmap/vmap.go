// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package vmap

import (
	"errors"
	"sync"
	"sync/atomic"
)

var ErrAlreadyClosed = errors.New("already closed")
var ErrConflictingWrite = errors.New("conflicting write")

type Option[K comparable, V any] func(*Map[K, V])

func WithForEach[K comparable, V any](fn func(func(K, V) error) error) Option[K, V] {
	return func(m *Map[K, V]) {
		m.fn.forEach = fn
	}
}

func WithCommit[K comparable, V any](fn func(map[K]V) error) Option[K, V] {
	return func(m *Map[K, V]) {
		m.fn.commit = fn
	}
}

func New[K comparable, V any](opts ...Option[K, V]) *Map[K, V] {
	m := new(Map[K, V])
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Map is a versioned map.
type Map[K comparable, V any] struct {
	mu    sync.Mutex
	stack []map[K]V
	refs  []int
	fn    struct {
		forEach func(func(K, V) error) error
		commit  func(map[K]V) error
	}
}

func (v *Map[K, V]) View() *View[K, V] {
	v.mu.Lock()
	l := len(v.stack) - 1
	if l < 0 {
		l = 0
		v.stack = append(v.stack, map[K]V{})
		v.refs = append(v.refs, 0)
	}
	v.refs[l]++
	v.mu.Unlock()

	u := new(View[K, V])
	u.vm = v
	u.level = l
	u.mine = map[K]V{}
	return u
}

func (v *Map[K, V]) commit() error {
	if v.fn.commit == nil {
		return nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	// TODO: I think it's ok to commit to disk as long as the ref count on the
	// base layer is zero, regardless of whether there are higher layers
	if len(v.stack) != 1 || v.refs[0] > 0 {
		return nil
	}

	values := v.stack[0]
	v.stack[0] = nil
	v.stack = v.stack[:0]
	v.refs = v.refs[:0]
	return v.fn.commit(values)
}

func (v *Map[K, V]) get(level int, k K) (V, bool) {
	for i := level; i >= 0; i-- {
		if v, ok := v.stack[i][k]; ok {
			return v, true
		}
	}

	var z V
	return z, false
}

func (v *Map[K, V]) forEach(level int, seen map[K]bool, fn func(K, V) error) error {
	for i := level; i >= 0; i-- {
		for k, v := range v.stack[i] {
			if seen[k] {
				continue
			}
			seen[k] = true
			err := fn(k, v)
			if err != nil {
				return err
			}
		}
	}

	if v.fn.forEach != nil {
		return v.fn.forEach(func(k K, v V) error {
			if seen[k] {
				return nil
			}
			seen[k] = true
			return fn(k, v)
		})
	}

	return nil
}

func (v *Map[K, V]) release(level int, values map[K]V) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Check for a conflict
	if level+1 < len(v.stack) {
		for _, m := range v.stack[level+1:] {
			// Iterate over the smaller of the two
			n := values
			if len(m) < len(n) {
				m, n = n, m
			}
			for k := range n {
				if _, ok := m[k]; ok {
					return ErrConflictingWrite
				}
			}
		}
	}

	// Decrement the ref-count of the specified level
	v.refs[level]--

	i := len(v.stack) - 1
	switch {
	case len(values) > 0:
		// If the commit set has values, append to the stack and compact
		v.stack = append(v.stack, values)
		v.refs = append(v.refs, 0)
		i++

	case v.refs[i] > 0:
		// If the latest level has refs and the commit set is empty, do nothing
		return nil

	default:
		// If the latest level has no refs, compact
	}

	// Find the lowest level with no refs
	for i > 0 && v.refs[i-1] == 0 {
		i--
	}

	// Compact
	m := v.stack[i]
	for _, l := range v.stack[i+1:] {
		if len(m) == 0 {
			v.stack[i], m = l, l
			continue
		}
		for k, v := range l {
			m[k] = v
		}
	}

	// Trim
	i++
	clear(v.stack[i:]) // for GC
	v.stack = v.stack[:i]
	v.refs = v.refs[:i]
	return nil
}

// View is a view into a specific version of a vmap.
type View[K comparable, V any] struct {
	vm    *Map[K, V]
	level int
	mine  map[K]V
	done  atomic.Bool
}

func (v *View[K, V]) Get(k K) (V, bool) {
	if v, ok := v.mine[k]; ok {
		return v, true
	}
	return v.vm.get(v.level, k)
}

func (v *View[K, V]) ForEach(fn func(K, V) error) error {
	seen := map[K]bool{}
	for k, v := range v.mine {
		seen[k] = true
		err := fn(k, v)
		if err != nil {
			return err
		}
	}

	return v.vm.forEach(v.level, seen, fn)
}

func (v *View[K, V]) Put(k K, u V) {
	v.mine[k] = u
}

func (v *View[K, V]) Discard() {
	if !v.done.CompareAndSwap(false, true) {
		return
	}
	_ = v.vm.release(v.level, nil)
}

func (v *View[K, V]) Commit() error {
	if !v.done.CompareAndSwap(false, true) {
		return ErrAlreadyClosed
	}
	err := v.vm.release(v.level, v.mine)
	if err != nil {
		return err
	}
	return v.vm.commit()
}
