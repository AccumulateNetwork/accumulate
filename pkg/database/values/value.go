// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package values

//lint:file-ignore U1000 false positive

import (
	"io"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

// debug is a bit field for enabling debug log messages
// nolint
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

type value[T any] struct {
	logger       logging.OptionalLogger
	store        database.Store
	key          *database.Key
	name         string
	status       valueStatus
	value        encodableValue[T]
	allowMissing bool
	version      int
}

var _ database.Value = (*value[*wrappedValue[uint64]])(nil)

// NewValue returns a new value using the given encodable value.
func newValue[T any](logger log.Logger, store database.Store, key *database.Key, name string, allowMissing bool, ev encodableValue[T]) *value[T] {
	v := &value[T]{}
	v.logger.L = logger
	v.store = store
	v.key = key
	v.name = name
	v.value = ev
	v.allowMissing = allowMissing
	v.status = valueUndefined
	return v
}

func (v *value[T]) Key() *database.Key { return v.key }

// Get loads the value, unmarshalling it if necessary. If IsDirty returns true,
// Get is guaranteed to succeed.
func (v *value[T]) Get() (u T, err error) {
	// Do we already have the value?
	switch v.status {
	case valueNotFound:
		return zero[T](), errors.NotFound.WithFormat("%s not found", v.name)

	case valueClean, valueDirty:
		return v.value.getValue(), nil
	}

	// Logging
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

	// Get the value
	err = v.store.GetValue(v.key, v)
	switch {
	case err == nil:
		// Found it
		v.status = valueClean
		return v.value.getValue(), nil

	case !errors.Is(err, storage.ErrNotFound):
		// Unknown error
		return zero[T](), errors.UnknownError.Wrap(err)

	case v.allowMissing:
		// Initialize to an empty value
		v.value.setNew()
		v.status = valueClean
		return v.value.getValue(), nil

	default:
		// Not found
		v.status = valueNotFound
		return zero[T](), errors.NotFound.WithFormat("%s not found", v.name)
	}
}

// GetAs loads the value, coerces it to the target type, and assigns it to the
// target. The target must be a non-nil pointer to T.
func (v *value[T]) GetAs(target interface{}) error {
	u, err := v.Get()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = encoding.SetPtr(u, target)
	return errors.WrongType.Wrap(err)
}

// Put stores the value.
func (v *value[T]) Put(u T) error {
	switch debug & (debugPut | debugPutValue) {
	case debugPut | debugPutValue:
		v.logger.Debug("Put", "key", v.key, "value", u)
	case debugPut:
		v.logger.Debug("Put", "key", v.key)
	case debugPutValue:
		v.logger.Debug("Put", "value", u)
	}

	// Required for proper versioning
	if v.status == valueUndefined {
		_, err := v.Get()
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			return errors.UnknownError.Wrap(err)
		}
	}

	v.value.setValue(u)
	v.status = valueDirty
	v.version++
	return nil
}

// IsDirty implements Record.IsDirty.
func (v *value[T]) IsDirty() bool {
	if v == nil {
		return false
	}
	return v.status == valueDirty
}

// Commit implements Record.Commit.
func (v *value[T]) Commit() error {
	if v == nil || v.status != valueDirty {
		return nil
	}

	err := v.store.PutValue(v.key, v)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	return nil
}

// Resolve implements Record.Resolve.
func (v *value[T]) Resolve(key *database.Key) (database.Record, *database.Key, error) {
	if key.Len() == 0 {
		return v, nil, nil
	}
	return nil, nil, errors.InternalError.With("bad key for value")
}

func (v *value[T]) Walk(opts database.WalkOptions, fn database.WalkFunc) error {
	if opts.Modified && !v.IsDirty() {
		return nil
	}

	// Avoid extra work if the record has been loaded
	if v.status == valueNotFound {
		return nil
	}
	switch v.status {
	case valueNotFound:
		// Record does not exist
		return nil

	case valueDirty:
		// Record has been modified - walk it
		_, err := fn(v)
		return err

	case valueClean:
		// Record has been loaded
		if v.allowMissing {
			// If a missing value is allowed, valueClean can't be trusted
			break
		}

		// Walk it
		_, err := fn(v)
		return err
	}

	// Check if the record exists
	err := v.store.GetValue(v.key, v)
	switch {
	case err == nil:
		// Record exists - walk it
		_, err := fn(v)
		return err

	case errors.Is(err, errors.NotFound):
		// Record does not exist
		return nil

	default:
		return errors.UnknownError.Wrap(err)
	}
}

// GetValue loads the value.
func (v *value[T]) GetValue() (encoding.BinaryValue, int, error) {
	_, err := v.Get()
	if err != nil {
		return nil, 0, errors.UnknownError.Wrap(err)
	}
	return v.value, v.version, nil
}

// LoadValue sets the value from the reader. If put is false, the value will be
// copied. If put is true, the value will be marked dirty.
func (v *value[T]) LoadValue(value database.Value, put bool) error {
	uv, version, err := value.GetValue()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Warn if a value is modified incorrectly - except for the BPT since it
	// doesn't follow the rules
	if put && version <= v.version && !isBptValue(v.key) {
		v.logger.Error("Conflicting values written from concurrent batches", "key", v.key)
	}
	v.version = version

	u, ok := uv.(encodableValue[T])
	if !ok {
		return errors.InternalError.WithFormat("store %s: invalid value: want %T, got %T", v.name, (encodableValue[T])(nil), uv)
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

func isBptValue(key *database.Key) bool {
	if key.Len() < 2 {
		return false
	}
	s, ok := key.Get(key.Len() - 2).(string)
	return ok && s == "BPT"
}

// LoadBytes unmarshals the value from bytes.
func (v *value[T]) LoadBytes(data []byte, put bool) error {
	err := v.value.UnmarshalBinary(data)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	if put {
		v.version++
		v.status = valueDirty
	} else {
		v.status = valueClean
	}
	return nil
}

type ptrBinaryValue[T any] interface {
	*T
	encoding.BinaryValue
}

type structValue[T any, PT ptrBinaryValue[T]] struct {
	value PT
}

// Struct returns an encodable value for the given encodable struct-type.
func Struct[T any, PT ptrBinaryValue[T]]() encodableValue[PT] {
	return new(structValue[T, PT])
}

func (v *structValue[T, PT]) getValue() PT  { return v.value }
func (v *structValue[T, PT]) setValue(u PT) { v.value = u }
func (v *structValue[T, PT]) setNew()       { v.value = new(T) }

func (v *structValue[T, PT]) copyValue() PT {
	if v.value == nil {
		return nil
	}
	return v.value.CopyAsInterface().(PT)
}

func (v *structValue[T, PT]) CopyAsInterface() interface{} {
	u := new(structValue[T, PT])
	u.value = v.copyValue()
	return u
}

func (v *structValue[T, PT]) MarshalBinary() (data []byte, err error) {
	data, err = v.value.MarshalBinary()
	return data, errors.UnknownError.Wrap(err)
}

func (v *structValue[T, PT]) UnmarshalBinary(data []byte) error {
	var u PT = new(T)
	err := u.UnmarshalBinary(data)
	if err == nil {
		v.value = u
	}
	return errors.UnknownError.Wrap(err)
}

func (v *structValue[T, PT]) UnmarshalBinaryFrom(rd io.Reader) error {
	var u PT = new(T)
	err := u.UnmarshalBinaryFrom(rd)
	if err == nil {
		v.value = u
	}
	return errors.UnknownError.Wrap(err)
}

type unionValue[T encoding.BinaryValue] struct {
	value     T
	unmarshal func([]byte) (T, error)
}

// Union returns an encodable value for the given encodable union type.
func Union[T encoding.BinaryValue](unmarshal func([]byte) (T, error)) encodableValue[T] {
	return &unionValue[T]{unmarshal: unmarshal}
}

// UnionFactory curries Union.
func UnionFactory[T encoding.BinaryValue](unmarshal func([]byte) (T, error)) func() encodableValue[T] {
	return func() encodableValue[T] { return &unionValue[T]{unmarshal: unmarshal} }
}

func (v *unionValue[T]) getValue() T  { return v.value }
func (v *unionValue[T]) setValue(u T) { v.value = u }
func (v *unionValue[T]) setNew()      { v.value = zero[T]() }

func (v *unionValue[T]) copyValue() T {
	if interface{}(v.value) == nil {
		return zero[T]()
	}
	return v.value.CopyAsInterface().(T)
}

func (v *unionValue[T]) CopyAsInterface() interface{} {
	u := new(unionValue[T])
	u.unmarshal = v.unmarshal
	u.value = v.copyValue()
	return u
}

func (v *unionValue[T]) MarshalBinary() (data []byte, err error) {
	data, err = v.value.MarshalBinary()
	return data, errors.UnknownError.Wrap(err)
}

func (v *unionValue[T]) UnmarshalBinary(data []byte) error {
	u, err := v.unmarshal(data)
	if err == nil {
		v.value = u
	}
	return errors.UnknownError.Wrap(err)
}

func (v *unionValue[T]) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	err = v.UnmarshalBinary(data)
	return errors.UnknownError.Wrap(err)
}
