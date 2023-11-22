// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"encoding/json"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

type ServiceOrRef[T interface {
	~*U
	Service
	encoding.BinaryValue
	Copy() T
	Equal(T) bool
}, U any] struct {
	reference *string
	service   T
}

func (s *ServiceOrRef[T, U]) refOr(def string) string {
	if s != nil && s.reference != nil {
		return *s.reference
	}
	return def
}

func (s *ServiceOrRef[T, U]) Required(def string) []ioc.Requirement {
	if s == nil {
		return nil
	}
	if s.service == nil {
		return []ioc.Requirement{
			{Descriptor: ioc.NewDescriptorOf[keyvalue.Beginner](s.refOr(def))},
		}
	}
	return s.service.Requires()
}

func (s *ServiceOrRef[T, U]) MarshalJSON() ([]byte, error) {
	var z T
	if s.service != z {
		return json.Marshal(s.service)
	}
	return json.Marshal(s.reference)
}

func (s *ServiceOrRef[T, U]) UnmarshalJSON(b []byte) error {
	if json.Unmarshal(b, &s.reference) == nil {
		return nil
	}
	ss := T(new(U))
	err := ss.UnmarshalBinary(b)
	if err != nil {
		return err
	}
	s.service = ss
	return nil
}

func (s *ServiceOrRef[T, U]) Copy() *ServiceOrRef[T, U] {
	if s.service != nil {
		return &ServiceOrRef[T, U]{service: s.service.Copy()}
	}
	return s // Reference is immutable
}

func (s *ServiceOrRef[T, U]) Equal(t *ServiceOrRef[T, U]) bool {
	if s.reference != t.reference {
		return false
	}
	if s.service == t.service {
		return true
	}
	if s.service == nil || t.service == nil {
		return false
	}
	return s.service.Equal(t.service)
}
