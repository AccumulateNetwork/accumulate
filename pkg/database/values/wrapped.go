// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package values

import (
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

//lint:file-ignore U1000 false positive

type wrappedValue[T any] struct {
	value T
	*wrapperFuncs[T]
}

// Wrapped returns an encodable value for the given type using the given wrapper
// functions.
func Wrapped[T any](funcs *wrapperFuncs[T]) encodableValue[T] {
	return &wrappedValue[T]{wrapperFuncs: funcs}
}

// WrappedFactory curries Wrapped.
func WrappedFactory[T any](funcs *wrapperFuncs[T]) func() encodableValue[T] {
	return func() encodableValue[T] { return &wrappedValue[T]{wrapperFuncs: funcs} }
}

func (v *wrappedValue[T]) getValue() T  { return v.value }
func (v *wrappedValue[T]) setValue(u T) { v.value = u }
func (v *wrappedValue[T]) setNew()      { v.value = zero[T]() }
func (v *wrappedValue[T]) copyValue() T { return v.copy(v.value) }

func (v *wrappedValue[T]) MarshalBinary() ([]byte, error) {
	data, err := v.marshal(v.value)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return data, nil
}

func (v *wrappedValue[T]) UnmarshalBinary(data []byte) error {
	u, err := v.unmarshal(data)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	v.value = u
	return nil
}

func (v *wrappedValue[T]) CopyAsInterface() interface{} {
	w := *v
	w.value = v.copy(v.value)
	return &w
}

func (v *wrappedValue[T]) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = v.UnmarshalBinary(data)
	return errors.UnknownError.Wrap(err)
}
