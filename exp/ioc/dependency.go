// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package ioc

import (
	"errors"
	"reflect"
)

type Dependency[A, B any] struct {
	ns       func(A) string
	optional bool
}

func Needs[B, A any](ns func(A) string) *Dependency[A, B] {
	return &Dependency[A, B]{ns: ns}
}

func Wants[B, A any](ns func(A) string) *Dependency[A, B] {
	return &Dependency[A, B]{ns: ns, optional: true}
}

func Provides[B, A any](ns func(A) string) *Dependency[A, B] {
	return &Dependency[A, B]{ns: ns}
}

func (d *Dependency[A, B]) Type() reflect.Type {
	return reflect.TypeOf(new(B)).Elem()
}

func (d *Dependency[A, B]) describe(a A) Descriptor {
	return NewDescriptorOf[B](d.ns(a))
}

func (d *Dependency[A, B]) Requirement(a A) Requirement {
	return Requirement{
		Descriptor: d.describe(a),
		Optional:   d.optional,
	}
}

func (d *Dependency[A, B]) Provided(a A) Provided {
	return Provided{
		Descriptor: d.describe(a),
	}
}

func (d *Dependency[A, B]) Register(reg Registry, a A, service B) error {
	return reg.Register(d.describe(a), service)
}

func (d *Dependency[A, B]) Get(reg Registry, a A) (B, error) {
	v, err := Get[B](reg, d.ns(a))
	if d.optional && errors.Is(err, ErrNotRegistered) {
		return v, nil
	}
	return v, err
}
