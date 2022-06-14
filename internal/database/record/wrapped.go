package record

import (
	"io"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
)

type wrapperType[T any] interface {
	encoding.BinaryValue
	getValue() T
	setValue(T)
}

type Wrapped[T any] struct {
	Value[wrapperType[T]]
}

func NewWrapped[T any](store Store, key Key, namefmt string, allowMissing bool, new func() wrapperType[T]) *Wrapped[T] {
	v := &Wrapped[T]{}
	v.Value = *NewValue(store, key, namefmt, allowMissing, new)
	return v
}

func (v *Wrapped[T]) Get() (T, error) {
	w, err := v.Value.Get()
	if err != nil {
		return zero[T](), errors.Wrap(errors.StatusUnknown, err)
	}
	return w.getValue(), nil
}

func (v *Wrapped[T]) GetAs(target interface{}) error {
	u, err := v.Get()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = encoding.SetPtr(u, target)
	return errors.Wrap(errors.StatusUnknown, err)
}

func (v *Wrapped[T]) Put(u T) error {
	w := v.new()
	w.setValue(u)
	err := v.Value.Put(w)
	return errors.Wrap(errors.StatusUnknown, err)
}

func (v *Wrapped[T]) IsDirty() bool {
	if v == nil {
		return false
	}
	return v.Value.IsDirty()
}

func (v *Wrapped[T]) Commit() error {
	if v == nil {
		return nil
	}
	err := v.Value.Commit()
	return errors.Wrap(errors.StatusUnknown, err)
}

type Wrapper[T any] struct {
	value T
	*wrapperFuncs[T]
}

func NewWrapper[T any](funcs *wrapperFuncs[T]) func() wrapperType[T] {
	return func() wrapperType[T] {
		w := &Wrapper[T]{}
		w.wrapperFuncs = funcs
		return w
	}
}

func (v *Wrapper[T]) getValue() T  { return v.value }
func (v *Wrapper[T]) setValue(u T) { v.value = u }

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
