package database

import (
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type valueStatus int

const (
	valueUndefined valueStatus = iota
	valueNotFound
	valueClean
	valueDirty
)

type Value[T encoding.BinaryValue] struct {
	store        recordStore
	key          recordKey
	name         string
	new          func() T
	status       valueStatus
	value        T
	allowMissing bool
}

func newValue[T encoding.BinaryValue](store recordStore, key recordKey, namefmt string, allowMissing bool, new func() T) *Value[T] {
	v := &Value[T]{}
	v.store = store
	v.key = key
	if strings.ContainsRune(namefmt, '%') {
		v.name = fmt.Sprintf(namefmt, key...)
	} else {
		v.name = namefmt
	}
	v.new = new
	v.allowMissing = allowMissing
	v.status = valueUndefined
	return v
}

func zero[T any]() (z T) { return z }

func (v *Value[T]) Get() (T, error) {
	switch v.status {
	case valueNotFound:
		return zero[T](), errors.NotFound("%s not found", v.name)

	case valueClean, valueDirty:
		return v.value, nil
	}

	u := v.new()
	err := v.store.get(v.key, u)
	switch {
	case err == nil:
		v.value = u
		v.status = valueClean
		return v.value, nil

	case !errors.Is(err, storage.ErrNotFound):
		return zero[T](), errors.Wrap(errors.StatusUnknown, err)

	case v.allowMissing:
		v.value = u
		v.status = valueClean
		return v.value, nil

	default:
		v.status = valueNotFound
		return zero[T](), errors.NotFound("%s not found", v.name)
	}
}

func (v *Value[T]) GetAs(target interface{}) error {
	u, err := v.Get()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = encoding.SetPtr(u, target)
	return errors.Wrap(errors.StatusUnknown, err)
}

func (v *Value[T]) Put(u T) error {
	v.value = u
	v.status = valueDirty
	return nil
}

func (v *Value[T]) isDirty() bool {
	if v == nil {
		return false
	}
	return v.status == valueDirty
}

func (v *Value[T]) commit() error {
	if v == nil || v.status != valueDirty {
		return nil
	}

	err := v.store.put(v.key, v.value)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	return nil
}

func (v *Value[T]) resolve(key recordKey) (record, recordKey, error) {
	return nil, nil, errors.New(errors.StatusInternalError, "bad key for value")
}

func (v *Value[T]) get(value encoding.BinaryValue) error {
	u, err := v.Get()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	w := u.CopyAsInterface()
	err = encoding.SetPtr(w, value)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	return nil
}

func (v *Value[T]) put(value encoding.BinaryValue) error {
	u, ok := value.(T)
	if !ok {
		return errors.Format(errors.StatusInternalError, "store %s: invalid value: want %T, got %T", v.name, v.new(), value)
	}

	v.value = u
	return nil
}
