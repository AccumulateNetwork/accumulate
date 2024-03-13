// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import (
	"errors"
	"strings"
	"sync"
)

type UnionRegistry[T interface {
	comparable
	EnumValueGetter
}, V interface {
	Type() T
	UnionValue
}] struct {
	mu      sync.RWMutex
	new     map[T]func() V
	equal   map[T]func(V, V) bool
	name    map[T]string
	byName  map[string]T
	byValue map[uint64]T
}

func (r *UnionRegistry[T, V]) Register(str string, new func() V, equal func(V, V) bool) error {
	typ := new().Type()

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.new == nil {
		r.new = map[T]func() V{}
		r.equal = map[T]func(V, V) bool{}
		r.name = map[T]string{}
		r.byName = map[string]T{}
		r.byValue = map[uint64]T{}
	}

	lstr := strings.ToLower(str)
	if _, ok := r.new[typ]; ok {
		return errors.New("type is already registered")
	}
	if _, ok := r.byName[lstr]; ok {
		return errors.New("type name is already registered")
	}
	if _, ok := r.byValue[typ.GetEnumValue()]; ok {
		return errors.New("type value is already registered")
	}

	r.new[typ] = new
	r.equal[typ] = equal
	r.name[typ] = str
	r.byName[lstr] = typ
	r.byValue[typ.GetEnumValue()] = typ
	return nil
}

func (r *UnionRegistry[T, V]) New(typ T) (V, bool) {
	r.mu.RLock()
	f, ok := r.new[typ]
	r.mu.RUnlock()
	if !ok {
		var z V
		return z, false
	}
	return f(), true
}

func (r *UnionRegistry[T, V]) Equal(u, v V) bool {
	r.mu.RLock()
	fn, ok := r.equal[u.Type()]
	r.mu.RUnlock()
	return ok && fn(u, v)
}

func (r *UnionRegistry[T, V]) TypeName(typ T) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.name[typ]
	return s, ok
}

func (r *UnionRegistry[T, V]) TypeByName(s string) (T, bool) {
	s = strings.ToLower(s)
	r.mu.RLock()
	defer r.mu.RUnlock()
	typ, ok := r.byName[s]
	return typ, ok
}

func (r *UnionRegistry[T, V]) TypeByValue(v uint64) (T, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	typ, ok := r.byValue[v]
	return typ, ok
}
