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
}

func (v *vmap[K, V]) release(level int, values map[K]V) {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Decrement the ref-count of the specified level
	v.refs[level]--

	// If the latest layer has no refs, put directly into it
	latest := len(v.stack) - 1
	if latest >= 0 && v.refs[latest] == 0 {
		m := v.stack[latest]
		if len(m) == 0 {
			v.stack[latest] = values
		} else {
			for k, v := range values {
				m[k] = v
			}
		}
	} else {
		// Otherwise add a new layer
		v.stack = append(v.stack, values)
		v.refs = append(v.refs, 0)
	}

	// Find the lowest level with no refs
	lowest := len(v.refs)
	for lowest > 0 && v.refs[lowest-1] == 0 {
		lowest--
	}

	// Compact
	m := v.stack[lowest]
	for _, l := range v.stack[lowest+1:] {
		for k, v := range l {
			m[k] = v
		}
	}
	v.stack = v.stack[:lowest+1]
	v.refs = v.refs[:lowest+1]
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
	for i := v.level; i >= 0; i-- {
		if v, ok := v.vm.stack[i][k]; ok {
			return v, true
		}
	}
	var z V
	return z, false
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

	for i := v.level; i >= 0; i-- {
		for k, v := range v.vm.stack[i] {
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
	return nil
}

func (v *vmapView[K, V]) Put(k K, u V) {
	v.mine[k] = u
}

func (v *vmapView[K, V]) Discard() {
	v.done.Do(func() { v.vm.release(v.level, nil) })
}

func (v *vmapView[K, V]) Commit() error {
	v.done.Do(func() { v.vm.release(v.level, v.mine) })
	return nil
}
