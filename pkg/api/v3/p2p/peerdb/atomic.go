// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package peerdb

import (
	"encoding/json"
	"sync/atomic"

	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
)

type ptrValue[T any] interface {
	~*T
	Copy() *T
	Equal(*T) bool
	Compare(*T) int
}

type AtomicSlice[PT ptrValue[T], T any] atomic.Pointer[[]PT]

func (s *AtomicSlice[PT, T]) p() *atomic.Pointer[[]PT] {
	return (*atomic.Pointer[[]PT])(s)
}

func (s *AtomicSlice[PT, T]) Get(target PT) (PT, bool) {
	l := s.Load()
	i, ok := sortutil.Search(l, func(entry PT) int {
		return entry.Compare((*T)(target))
	})
	if ok {
		return l[i], true
	}
	return nil, false
}

func (s *AtomicSlice[PT, T]) Insert(target PT) PT {
	for {
		// Is the list empty?
		l := s.p().Load()
		if l == nil {
			m := []PT{target}
			if s.p().CompareAndSwap(l, &m) {
				return target
			}
			continue
		}

		// Is the element present?
		i, found := sortutil.Search(*l, func(entry PT) int {
			return entry.Compare((*T)(target))
		})
		if found {
			return (*l)[i]
		}

		// Copy the list and insert a new element
		m := make([]PT, len(*l)+1)
		copy(m, (*l)[:i])
		copy(m[i+1:], (*l)[i:])
		m[i] = target
		if s.p().CompareAndSwap(l, &m) {
			return m[i]
		}
	}
}

func (s *AtomicSlice[PT, T]) RemoveFunc(fn func(v PT) bool) {
	s.remove(func(l []PT) []int {
		var indices []int
		for _, t := range l {
			if !fn(t) {
				continue
			}
			i, found := sortutil.Search(l, func(entry PT) int {
				return entry.Compare((*T)(t))
			})
			if found {
				indices = append(indices, i)
			}
		}
		return indices
	})
}

func (s *AtomicSlice[PT, T]) Remove(targets ...PT) {
	s.remove(func(l []PT) []int {
		var indices []int
		for _, t := range targets {
			i, found := sortutil.Search(l, func(entry PT) int {
				return entry.Compare((*T)(t))
			})
			if found {
				indices = append(indices, i)
			}
		}
		return indices
	})
}

func (s *AtomicSlice[PT, T]) remove(fn func([]PT) []int) {
	for {
		// Is the list empty?
		l := s.p().Load()
		if l == nil {
			return
		}

		// Are the elements present?
		indices := fn(*l)
		if len(indices) == 0 {
			return
		}

		m := make([]PT, len(*l)-len(indices))
		n := copy(m, (*l)[:indices[0]])
		for i, j := range indices[1:] {
			i := indices[i]
			n += copy(m[n:], (*l)[i+1:j])
		}
		copy(m[n:], (*l)[indices[len(indices)-1]+1:])

		if s.p().CompareAndSwap(l, &m) {
			return
		}
	}
}

func (s *AtomicSlice[PT, T]) Load() []PT {
	if s == nil {
		return nil
	}
	p := s.p().Load()
	if p == nil {
		return nil
	}
	return *p
}

func (s *AtomicSlice[PT, T]) Store(v []PT) {
	s.p().Store(&v)
}

func (s *AtomicSlice[PT, T]) Copy() *AtomicSlice[PT, T] {
	v := s.Load()
	u := make([]PT, len(v))
	for i, v := range v {
		u[i] = v.Copy()
	}
	w := new(AtomicSlice[PT, T])
	w.Store(u)
	return w
}

func (s *AtomicSlice[PT, T]) Equal(t *AtomicSlice[PT, T]) bool {
	if s == t {
		return true
	}
	v, u := s.Load(), t.Load()
	if len(v) != len(u) {
		return false
	}
	for i, v := range v {
		if !v.Equal((*T)(u[i])) {
			return false
		}
	}
	return true
}

func (s *AtomicSlice[PT, T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Load())
}

func (s *AtomicSlice[PT, T]) UnmarshalJSON(b []byte) error {
	var l []PT
	err := json.Unmarshal(b, &l)
	s.Store(l)
	return err
}

type AtomicUint atomic.Uint64

func (v *AtomicUint) u() *atomic.Uint64 {
	return (*atomic.Uint64)(v)
}

func (v *AtomicUint) Load() uint64 {
	if v == nil {
		return 0
	}
	return v.u().Load()
}

func (v *AtomicUint) Store(u uint64) {
	v.u().Store(u)
}

func (v *AtomicUint) Add(delta uint64) {
	v.u().Add(delta)
}

func (v *AtomicUint) Copy() *AtomicUint {
	var u AtomicUint
	u.Store(v.Load())
	return &u
}

func (v *AtomicUint) Equal(u *AtomicUint) bool {
	return v.Load() == u.Load()
}

func (v *AtomicUint) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.Load())
}

func (v *AtomicUint) UnmarshalJSON(b []byte) error {
	var u uint64
	err := json.Unmarshal(b, &u)
	v.Store(u)
	return err
}
