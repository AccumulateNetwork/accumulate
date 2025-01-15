// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package ioc

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

var ErrAlreadyRegistered = errors.New("already registered")
var ErrNotRegistered = errors.New("not registered")

type Registry map[regKey]any

func (r Registry) Register(desc Descriptor, value any) error {
	key := desc2key(desc)
	if !reflect.TypeOf(value).AssignableTo(key.typ) {
		return errors.New("value does not match descriptor type")
	}

	// If there's no existing service, register it
	existing, ok := r[key]
	if !ok {
		r[key] = value
		return nil
	}

	// If the existing service is [Promised]...
	promised, ok := existing.(Promised)
	if !ok {
		return key.wrap(ErrAlreadyRegistered)
	}

	// Resolve the [Promised]
	err := promised.Resolve(value)
	if err != nil {
		return key.wrap(err)
	}

	// And replace it
	r[key] = value
	return nil
}

func (r Registry) Get(desc Descriptor) (any, bool) {
	v, ok := r[desc2key(desc)]
	return v, ok
}

func Register[T any](r Registry, namespace string, value T) error {
	desc := NewDescriptorOf[T](namespace)
	return r.Register(desc, value)
}

func Get[T any](r Registry, namespace string) (T, error) {
	desc := NewDescriptorOf[T](namespace)
	v, ok := r.Get(desc)
	if !ok {
		var z T
		return z, desc2key(desc).wrap(ErrNotRegistered)
	}
	return v.(T), nil
}

func ForEach[T any](r Registry, fn func(desc Descriptor, value T)) {
	for key, value := range r {
		if v, ok := value.(T); ok {
			fn(key, v)
		}
	}
}

type regKey struct {
	typ reflect.Type
	ns  string
}

func desc2key(desc Descriptor) regKey {
	if k, ok := desc.(regKey); ok {
		return k
	}
	return regKey{typ: desc.Type(), ns: strings.ToLower(desc.Namespace())}
}

func NewDescriptor(namespace string, typ reflect.Type) Descriptor {
	return regKey{typ, strings.ToLower(namespace)}
}

func NewDescriptorOf[T any](namespace string) Descriptor {
	typ := reflect.TypeOf(new(T)).Elem()
	return NewDescriptor(namespace, typ)
}

func (k regKey) wrap(err error) error {
	if k.ns == "" {
		return fmt.Errorf("%v %w", k.typ, err)
	}
	return fmt.Errorf("%v (namespace: %q) %w", k.typ, k.ns, err)
}

func (k regKey) Type() reflect.Type { return k.typ }
func (k regKey) Namespace() string  { return k.ns }
