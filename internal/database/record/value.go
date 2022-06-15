package record

import (
	"fmt"
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

type valueStatus int

const (
	valueUndefined valueStatus = iota
	valueNotFound
	valueClean
	valueDirty
)

type Value[T encoding.BinaryValue] struct {
	logger       logging.OptionalLogger
	store        Store
	key          Key
	name         string
	new          func() T
	status       valueStatus
	value        T
	allowMissing bool
}

var _ ValueReadWriter = (*Value[*Wrapper[uint64]])(nil)

func NewValue[T encoding.BinaryValue](logger log.Logger, store Store, key Key, namefmt string, allowMissing bool, new func() T) *Value[T] {
	v := &Value[T]{}
	v.logger.L = logger
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

func (v *Value[T]) Get() (u T, err error) {
	switch v.status {
	case valueNotFound:
		return zero[T](), errors.NotFound("%s not found", v.name)

	case valueClean, valueDirty:
		return v.value, nil
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

	err = v.store.LoadValue(v.key, v)
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
	switch debug & (debugPut | debugPutValue) {
	case debugPut | debugPutValue:
		v.logger.Debug("Put", "key", v.key, "value", u)
	case debugPut:
		v.logger.Debug("Put", "key", v.key)
	case debugPutValue:
		v.logger.Debug("Put", "value", u)
	}

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
	if len(key) == 0 {
		return v, nil, nil
	}
	return nil, nil, errors.New(errors.StatusInternalError, "bad key for value")
}

func (v *Value[T]) GetValue() (encoding.BinaryValue, error) {
	return v.Get()
}

func (v *Value[T]) from(value ValueReadWriter, read bool) error {
	vv, err := value.GetValue()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}
	u, ok := vv.(T)
	if !ok {
		return errors.Format(errors.StatusInternalError, "store %s: invalid value: want %T, got %T", v.name, v.new(), value)
	}

	if read {
		v.value = u.CopyAsInterface().(T)
		v.status = valueClean
	} else {
		v.value = u
		v.status = valueDirty
	}
	return nil
}

func (v *Value[T]) GetFrom(value ValueReadWriter) error {
	return v.from(value, true)
}
func (v *Value[T]) PutFrom(value ValueReadWriter) error {
	return v.from(value, false)
}

func (v *Value[T]) LoadFrom(data []byte) error {
	u := v.new()
	err := u.UnmarshalBinary(data)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	v.value = u
	v.status = valueClean
	return nil
}
