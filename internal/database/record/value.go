package record

import (
	"fmt"
	"io"
	"strings"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// debug is a bit field for enabling debug log messages
//nolint
const debug = 0 |
	// debugGet |
	// debugGetValue |
	// debugPut |
	// debugPutValue |
	0

const (
	// debugGet logs the key of Batch.getValue
	debugGet = 1 << iota
	// debugGetValue logs the value of Batch.getValue
	debugGetValue
	// debugPut logs the key of Batch.putValue
	debugPut
	// debugPutValue logs the value of Batch.putValue
	debugPutValue
)

type encodableValue[T any] interface {
	encoding.BinaryValue
	getValue() T
	setValue(T)
	setNew()
	copyValue() T
}

type valueStatus int

const (
	valueUndefined valueStatus = iota
	valueNotFound
	valueClean
	valueDirty
)

type Value[T any] struct {
	logger       logging.OptionalLogger
	store        Store
	key          Key
	name         string
	status       valueStatus
	value        encodableValue[T]
	allowMissing bool
}

var _ ValueReader = (*Value[*wrappedValue[uint64]])(nil)
var _ ValueWriter = (*Value[*wrappedValue[uint64]])(nil)

func NewValue[T any](logger log.Logger, store Store, key Key, namefmt string, allowMissing bool, value encodableValue[T]) *Value[T] {
	v := &Value[T]{}
	v.logger.L = logger
	v.store = store
	v.key = key
	if strings.ContainsRune(namefmt, '%') {
		v.name = fmt.Sprintf(namefmt, key...)
	} else {
		v.name = namefmt
	}
	v.value = value
	v.allowMissing = allowMissing
	v.status = valueUndefined
	return v
}

func (v *Value[T]) Key(i int) interface{} {
	return v.key[i]
}

func (v *Value[T]) Get() (u T, err error) {
	switch v.status {
	case valueNotFound:
		return zero[T](), errors.NotFound("%s not found", v.name)

	case valueClean, valueDirty:
		return v.value.getValue(), nil
	}

	switch debug & (debugGet | debugGetValue) {
	case debugGet | debugGetValue:
		defer func() {
			if err != nil {
				v.logger.Debug("Get", "key", v.key, "value", err)
			} else {
				v.logger.Debug("Get", "key", v.key, "value", u)
			}
		}()
	case debugGet:
		v.logger.Debug("Get", "key", v.key)
	case debugGetValue:
		defer func() {
			if err != nil {
				v.logger.Debug("Get", "error", err)
			} else {
				v.logger.Debug("Get", "value", u)
			}
		}()
	}

	err = v.store.GetValue(v.key, v)
	switch {
	case err == nil:
		v.status = valueClean
		return v.value.getValue(), nil

	case !errors.Is(err, storage.ErrNotFound):
		return zero[T](), errors.Wrap(errors.StatusUnknown, err)

	case v.allowMissing:
		v.value.setNew()
		v.status = valueClean
		return v.value.getValue(), nil

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
	switch debug & (debugPut | debugPutValue) {
	case debugPut | debugPutValue:
		v.logger.Debug("Put", "key", v.key, "value", u)
	case debugPut:
		v.logger.Debug("Put", "key", v.key)
	case debugPutValue:
		v.logger.Debug("Put", "value", u)
	}

	v.value.setValue(u)
	v.status = valueDirty

	// TODO AC-1761 remove commit
	return v.Commit()
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

	err := v.store.PutValue(v.key, v)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	return nil
}

func (v *Value[T]) Resolve(key Key) (Record, Key, error) {
	if len(key) == 0 {
		return v, nil, nil
	}
	return nil, nil, errors.New(errors.StatusInternalError, "bad key for value")
}

func (v *Value[T]) GetValue() (encoding.BinaryValue, error) {
	_, err := v.Get()
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}
	return v.value, nil
}

func (v *Value[T]) LoadValue(value ValueReader, put bool) error {
	uv, err := value.GetValue()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	u, ok := uv.(encodableValue[T])
	if !ok {
		return errors.Format(errors.StatusInternalError, "store %s: invalid value: want %T, got %T", v.name, (encodableValue[T])(nil), uv)
	}

	if put {
		v.value.setValue(u.getValue())
		v.status = valueDirty
	} else {
		v.value.setValue(u.copyValue())
		v.status = valueClean
	}
	return nil
}

func (v *Value[T]) LoadBytes(data []byte) error {
	err := v.value.UnmarshalBinary(data)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	v.status = valueClean
	return nil
}

type ptrBinaryValue[T any] interface {
	*T
	encoding.BinaryValue
}

type structValue[T any, PT ptrBinaryValue[T]] struct {
	value PT
}

func Struct[T any, PT ptrBinaryValue[T]]() encodableValue[PT] {
	return new(structValue[T, PT])
}

func (v *structValue[T, PT]) getValue() PT  { return v.value }
func (v *structValue[T, PT]) setValue(u PT) { v.value = u }
func (v *structValue[T, PT]) setNew()       { v.value = new(T) }
func (v *structValue[T, PT]) copyValue() PT { return v.CopyAsInterface().(PT) }

func (v *structValue[T, PT]) CopyAsInterface() interface{} {
	u := new(structValue[T, PT])
	u.value = v.copyValue()
	return u
}

func (v *structValue[T, PT]) MarshalBinary() (data []byte, err error) {
	data, err = v.value.MarshalBinary()
	return data, errors.Wrap(errors.StatusUnknown, err)
}

func (v *structValue[T, PT]) UnmarshalBinary(data []byte) error {
	var u PT = new(T)
	err := u.UnmarshalBinary(data)
	if err == nil {
		v.value = u
	}
	return errors.Wrap(errors.StatusUnknown, err)
}

func (v *structValue[T, PT]) UnmarshalBinaryFrom(rd io.Reader) error {
	var u PT = new(T)
	err := u.UnmarshalBinaryFrom(rd)
	if err == nil {
		v.value = u
	}
	return errors.Wrap(errors.StatusUnknown, err)
}

type unionValue[T encoding.BinaryValue] struct {
	value     T
	unmarshal func([]byte) (T, error)
}

func Union[T encoding.BinaryValue](unmarshal func([]byte) (T, error)) encodableValue[T] {
	return &unionValue[T]{unmarshal: unmarshal}
}

func UnionFactory[T encoding.BinaryValue](unmarshal func([]byte) (T, error)) func() encodableValue[T] {
	return func() encodableValue[T] { return &unionValue[T]{unmarshal: unmarshal} }
}

func (v *unionValue[T]) getValue() T  { return v.value }
func (v *unionValue[T]) setValue(u T) { v.value = u }
func (v *unionValue[T]) setNew()      { v.value = zero[T]() }
func (v *unionValue[T]) copyValue() T { return v.CopyAsInterface().(T) }

func (v *unionValue[T]) CopyAsInterface() interface{} {
	u := new(unionValue[T])
	u.value = v.value.CopyAsInterface().(T)
	return u
}

func (v *unionValue[T]) MarshalBinary() (data []byte, err error) {
	data, err = v.value.MarshalBinary()
	return data, errors.Wrap(errors.StatusUnknown, err)
}

func (v *unionValue[T]) UnmarshalBinary(data []byte) error {
	u, err := v.unmarshal(data)
	if err == nil {
		v.value = u
	}
	return errors.Wrap(errors.StatusUnknown, err)
}

func (v *unionValue[T]) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}
	err = v.UnmarshalBinary(data)
	return errors.Wrap(errors.StatusUnknown, err)
}
