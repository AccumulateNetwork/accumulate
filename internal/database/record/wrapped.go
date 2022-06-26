package record

import (
	"io"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
)

type wrappedValue[T any] struct {
	value T
	*wrapperFuncs[T]
}

// Wrapped returns an encodable value for the given type using the given wrapper
// functions.
func Wrapped[T any](funcs *wrapperFuncs[T]) EncodableValue[T] {
	return &wrappedValue[T]{wrapperFuncs: funcs}
}

// WrappedFactory curries Wrapped.
func WrappedFactory[T any](funcs *wrapperFuncs[T]) func() EncodableValue[T] {
	return func() EncodableValue[T] { return &wrappedValue[T]{wrapperFuncs: funcs} }
}

func (v *wrappedValue[T]) getValue() T  { return v.value }
func (v *wrappedValue[T]) setValue(u T) { v.value = u }
func (v *wrappedValue[T]) setNew()      { v.value = zero[T]() }
func (v *wrappedValue[T]) copyValue() T { return v.copy(v.value) }

func (v *wrappedValue[T]) MarshalBinary() ([]byte, error) {
	data, err := v.marshal(v.value)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	return data, nil
}

func (v *wrappedValue[T]) UnmarshalBinary(data []byte) error {
	u, err := v.unmarshal(data)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
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
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	err = v.UnmarshalBinary(data)
	return errors.Wrap(errors.StatusUnknownError, err)
}
