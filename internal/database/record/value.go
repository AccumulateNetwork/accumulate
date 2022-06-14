package record

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
	store        Store
	key          Key
	name         string
	new          func() T
	status       valueStatus
	value        T
	allowMissing bool
}

var _ ValueReadWriter = (*Value[*Wrapper[uint64]])(nil)

func NewValue[T encoding.BinaryValue](store Store, key Key, namefmt string, allowMissing bool, new func() T) *Value[T] {
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

func (v *Value[T]) Key(i int) interface{} {
	return v.key[i]
}

func (v *Value[T]) Get() (T, error) {
	switch v.status {
	case valueNotFound:
		return zero[T](), errors.NotFound("%s not found", v.name)

	case valueClean, valueDirty:
		return v.value, nil
	}

	err := v.store.LoadValue(v.key, v)
	switch {
	case err == nil:
		v.status = valueClean
		return v.value, nil

	case !errors.Is(err, storage.ErrNotFound):
		return zero[T](), errors.Wrap(errors.StatusUnknown, err)

	case v.allowMissing:
		v.value = v.new()
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

func (v *Value[T]) IsDirty() bool {
	if v == nil {
		return false
	}
	return v.status == valueDirty
}

func (v *Value[T]) Commit() error {
	if v == nil || v.status != valueDirty {
		return nil
	}

	err := v.store.StoreValue(v.key, v)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	return nil
}

func (v *Value[T]) Resolve(key Key) (Record, Key, error) {
	return nil, nil, errors.New(errors.StatusInternalError, "bad key for value")
}

func (v *Value[T]) GetValue() (encoding.BinaryValue, error) {
	return v.Get()
}

func (v *Value[T]) ReadFrom(value ValueReadWriter) error {
	vv, err := value.GetValue()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}
	u, ok := vv.(T)
	if !ok {
		return errors.Format(errors.StatusInternalError, "store %s: invalid value: want %T, got %T", v.name, v.new(), value)
	}

	v.value = u
	return nil
}

func (v *Value[T]) ReadFromBytes(data []byte) error {
	u := v.new()
	err := u.UnmarshalBinary(data)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	v.value = u
	return nil
}
