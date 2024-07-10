// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"sync"
)

// vmap is a versioned map.
type vmap[K comparable, V any] struct {
	mu    sync.Mutex
	stack []map[K]V
	refs  []int
	fn    struct {
		forEach func(func(K, V) error) error
		commit  func(map[K]V) error
	}
}

func (v *vmap[K, V]) View() *vmapView[K, V] {
	v.mu.Lock()
	l := len(v.stack) - 1
	if l < 0 {
		l = 0
		v.stack = append(v.stack, map[K]V{})
		v.refs = append(v.refs, 0)
	}
	v.refs[l]++
	v.mu.Unlock()

	u := new(vmapView[K, V])
	u.vm = v
	u.level = l
	u.mine = map[K]V{}
	return u
}

func (v *vmap[K, V]) commit() error {
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

func (v *vmap[K, V]) get(level int, k K) (V, bool) {
	for i := level; i >= 0; i-- {
		if v, ok := v.stack[i][k]; ok {
			return v, true
		}
	}

	var z V
	return z, false
}

func (v *vmap[K, V]) forEach(level int, seen map[K]bool, fn func(K, V) error) error {
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

func (v *vmap[K, V]) release(level int, values map[K]V) {
	v.mu.Lock()
	defer v.mu.Unlock()

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
		return

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
}

// vmapView is a view into a specific version of a vmap.
type vmapView[K comparable, V any] struct {
	vm    *vmap[K, V]
	level int
	mine  map[K]V
	done  sync.Once
}

func (v *vmapView[K, V]) Get(k K) (V, bool) {
	if v, ok := v.mine[k]; ok {
		return v, true
	}
	return v.vm.get(v.level, k)
}

func (v *vmapView[K, V]) ForEach(fn func(K, V) error) error {
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

func (v *vmapView[K, V]) Put(k K, u V) {
	v.mine[k] = u
}

func (v *vmapView[K, V]) Discard() {
	v.done.Do(func() { v.vm.release(v.level, nil) })
}

func (v *vmapView[K, V]) Commit() error {
	v.done.Do(func() { v.vm.release(v.level, v.mine) })
	return v.vm.commit()
}
