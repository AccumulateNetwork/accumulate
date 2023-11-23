// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"encoding/json"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

type baseRef[T comparable] struct {
	ref   *string
	value T
}

func (s *baseRef[T]) hasValue() bool {
	var z T
	return s != nil && s.value != z
}

func (s *baseRef[T]) refOr(def string) string {
	if s != nil && s.ref != nil {
		return *s.ref
	}
	return def
}

func (s *baseRef[T]) copyWith(copy func(T) T) *baseRef[T] {
	var z T
	if s.value != z {
		return &baseRef[T]{value: copy(s.value)}
	}
	return s // Reference is immutable
}

func (s *baseRef[T]) equalWith(t *baseRef[T], equal func(T, T) bool) bool {
	if s.ref != t.ref {
		return false
	}
	if s.value == t.value {
		return true
	}
	var z T
	if s.value == z || t.value == z {
		return false
	}
	return equal(s.value, t.value)
}

func (s *baseRef[T]) marshal() ([]byte, error) {
	var z T
	if s.value != z {
		return json.Marshal(s.value)
	}
	return json.Marshal(s.ref)
}

func (s *baseRef[T]) unmarshal(b []byte) error {
	if json.Unmarshal(b, &s.ref) == nil {
		return nil
	}
	return json.Unmarshal(b, &s.value)
}

func (s *baseRef[T]) unmarshalWith(b []byte, unmarshal func([]byte) (T, error)) error {
	if json.Unmarshal(b, &s.ref) == nil {
		return nil
	}
	ss, err := unmarshal(b)
	if err != nil {
		return err
	}
	s.value = ss
	return nil
}

type ServiceOrRef[T serviceType[T]] baseRef[T]

type serviceType[T any] interface {
	Service
	encoding.BinaryValue
	Copy() T
	Equal(T) bool
	comparable
}

func ServiceValue[T serviceType[T]](service T) *ServiceOrRef[T] {
	return &ServiceOrRef[T]{value: service}
}

func ServiceReference[T serviceType[T]](ref string) *ServiceOrRef[T] {
	return &ServiceOrRef[T]{ref: &ref}
}

func (s *ServiceOrRef[T]) base() *baseRef[T] {
	return (*baseRef[T])(s)
}

func (s *ServiceOrRef[T]) RequiresOr(def ...ioc.Requirement) []ioc.Requirement {
	if s.base().hasValue() {
		return s.value.Requires()
	}
	return def
}

func (s *ServiceOrRef[T]) Copy() *ServiceOrRef[T] {
	return (*ServiceOrRef[T])(s.base().copyWith((T).Copy))
}

func (s *ServiceOrRef[T]) Equal(t *ServiceOrRef[T]) bool {
	return s.base().equalWith(t.base(), (T).Equal)
}

func (s *ServiceOrRef[T]) MarshalJSON() ([]byte, error) {
	return s.base().marshal()
}

func (s *ServiceOrRef[T]) UnmarshalJSON(b []byte) error {
	return s.base().unmarshal(b)
}
