// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"fmt"
	"reflect"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type Service interface {
	Type() ServiceType
	CopyAsInterface() any

	start(inst *Instance) error
}

func registerService[T any](inst *Instance, name string, service any) error {
	if inst.services == nil {
		inst.services = map[reflect.Type]map[string]any{}
	}

	typ := reflect.TypeOf(new(T)).Elem()
	if inst.services[typ] == nil {
		inst.services[typ] = map[string]any{}
	}

	if _, ok := service.(T); !ok {
		panic(fmt.Errorf("%T is not %v", service, typ))
	}

	m := inst.services[typ]
	if m[name] != nil {
		return errors.InternalError.WithFormat("service %s (%v) already registered", name, typ)
	}
	m[name] = service
	return nil
}

func getService[T any](inst *Instance, name string) (T, error) {
	typ := reflect.TypeOf(new(T)).Elem()
	v, ok := inst.services[typ][name]
	if !ok {
		var z T
		return z, errors.InternalError.WithFormat("service %s (%v) not registered", name, typ)
	}
	return v.(T), nil
}
