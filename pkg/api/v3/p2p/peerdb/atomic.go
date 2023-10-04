// Copyright 2023 The Accumulate Authors
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
