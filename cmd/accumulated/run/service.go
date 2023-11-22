// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"reflect"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type Service interface {
	Type() ServiceType
	CopyAsInterface() any

	needs() []ServiceDescriptor
	provides() []ServiceDescriptor
	start(inst *Instance) error
}

type ServiceDescriptor interface {
	Name() string
	Type() reflect.Type
	Optional() bool
}

type simpleDescriptor struct {
	name     string
	typ      reflect.Type
	optional bool
}

func (s *simpleDescriptor) Name() string       { return s.name }
func (s *simpleDescriptor) Type() reflect.Type { return s.typ }
func (s *simpleDescriptor) Optional() bool     { return s.optional }

type dependency[A, B any] struct {
	name     func(A) string
	optional bool
}

type dependencyFor[A, B any] struct {
	*dependency[A, B]
	a A
}

func needs[B, A any](name func(A) string) *dependency[A, B] {
	return &dependency[A, B]{name: name}
}

func wants[B, A any](name func(A) string) *dependency[A, B] {
	return &dependency[A, B]{name: name, optional: true}
}

func provides[B, A any](name func(A) string) *dependency[A, B] {
	return &dependency[A, B]{name: name}
}

func (d *dependency[A, B]) Type() reflect.Type {
	return reflect.TypeOf(new(B)).Elem()
}

func (d *dependency[A, B]) Optional() bool {
	return d.optional
}

func (d *dependency[A, B]) with(a A) dependencyFor[A, B] {
	return dependencyFor[A, B]{d, a}
}

func (d dependencyFor[A, B]) Name() string {
	return d.name(d.a)
}

func (d *dependency[A, B]) register(inst *Instance, a A, service B) error {
	if inst.services == nil {
		inst.services = map[serviceKey]any{}
	}

	key := desc2key(d.with(a))
	if inst.services[key] != nil {
		return errors.InternalError.WithFormat("service %s (%v) already registered", key.Name, key.Type)
	}
	inst.services[key] = service
	return nil
}

func (d *dependency[A, B]) get(inst *Instance, a A) (B, error) {
	return getService[B](inst, d.with(a))
}

func getService[T any](inst *Instance, desc ServiceDescriptor) (T, error) {
	key := desc2key(desc)
	v, ok := inst.services[key]
	if ok {
		return v.(T), nil
	}
	var z T
	if desc.Optional() {
		return z, nil
	}
	return z, errors.InternalError.WithFormat("service %s (%v) not registered", key.Name, key.Type)
}
