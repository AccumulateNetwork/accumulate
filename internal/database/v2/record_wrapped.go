package database

import (
	"io"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
)

type Wrapped[T any] struct {
	Value[*Wrapper[T]]
}

func newWrapped[T any](store recordStore, key recordKey, namefmt string, allowMissing bool, new func() *Wrapper[T]) *Wrapped[T] {
	v := &Wrapped[T]{}
	v.Value = *newValue(store, key, namefmt, allowMissing, new)
	return v
}

func (v *Wrapped[T]) Get() (T, error) {
	w, err := v.Value.Get()
	if err != nil {
		return zero[T](), errors.Wrap(errors.StatusUnknown, err)
	}
	return w.value, nil
}

func (v *Wrapped[T]) Put(u T) error {
	w := v.new()
	w.value = u
	err := v.Value.Put(w)
	return errors.Wrap(errors.StatusUnknown, err)
}

func (v *Wrapped[T]) isDirty() bool {
	if v == nil {
		return false
	}
	return v.Value.isDirty()
}

func (v *Wrapped[T]) commit() error {
	if v == nil {
		return nil
	}
	err := v.Value.commit()
	return errors.Wrap(errors.StatusUnknown, err)
}

type Wrapper[T any] struct {
	value T
	*wrapperFuncs[T]
}

func newWrapper[T any](funcs *wrapperFuncs[T]) func() *Wrapper[T] {
	return func() *Wrapper[T] {
		w := &Wrapper[T]{}
		w.wrapperFuncs = funcs
		return w
	}
}

func (v *Wrapper[T]) MarshalBinary() ([]byte, error) {
	data, err := v.marshal(v.value)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	return data, nil
}

func (v *Wrapper[T]) UnmarshalBinary(data []byte) error {
	u, err := v.unmarshal(data)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	v.value = u
	return nil
}

func (v *Wrapper[T]) CopyAsInterface() interface{} {
	w := *v
	w.value = v.copy(v.value)
	return &w
}

func (v *Wrapper[T]) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = v.UnmarshalBinary(data)
	return errors.Wrap(errors.StatusUnknown, err)
}
