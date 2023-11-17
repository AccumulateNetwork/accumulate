// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"reflect"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type Service interface {
	Type() ServiceType
	CopyAsInterface() any

	needs() []ServiceDescriptor
	provides() []ServiceDescriptor
	start(inst *Instance) error
}

type ServiceDescriptor struct {
	Name string
	Type reflect.Type
}

type dependency[A, B any] struct {
	name func(A) string
}

func needs[B, A any](name func(A) string) *dependency[A, B] {
	return &dependency[A, B]{func(a A) string { return strings.ToLower(name(a)) }}
}

func provides[B, A any](name func(A) string) *dependency[A, B] {
	return &dependency[A, B]{func(a A) string { return strings.ToLower(name(a)) }}
}

func (d *dependency[A, B]) describe(a A) ServiceDescriptor {
	return ServiceDescriptor{
		Name: d.name(a),
		Type: reflect.TypeOf(new(B)).Elem(),
	}
}

func (d *dependency[A, B]) register(inst *Instance, a A, service B) error {
	if inst.services == nil {
		inst.services = map[ServiceDescriptor]any{}
	}

	desc := d.describe(a)
	if inst.services[desc] != nil {
		return errors.InternalError.WithFormat("service %s (%v) already registered", desc.Name, desc.Type)
	}
	inst.services[desc] = service
	return nil
}

func (d *dependency[A, B]) get(inst *Instance, a A) (B, error) {
	desc := d.describe(a)
	v, ok := inst.services[desc]
	if !ok {
		var z B
		return z, errors.InternalError.WithFormat("service %s (%v) not registered", desc.Name, desc.Type)
	}
	return v.(B), nil
}
