package database

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// Value reads and writes a value.
type ValueAs[T encoding.BinaryValue] struct {
	batch *Batch
	key   storage.Key
	new   func() T
}

// Key returns the value's storage key.
func (v *ValueAs[T]) Key() storage.Key {
	return v.key
}

// Get loads the value.
func (v *ValueAs[T]) Get() (T, error) {
	var zero T
	value := v.new()
	err := v.batch.getValuePtr(v.key, value, &value, false)
	if err != nil {
		return zero, errors.Wrap(errors.StatusUnknown, err)
	}

	return value, nil
}

// Put stores the value.
func (v *ValueAs[T]) Put(value T) error {
	v.batch.putValue(v.key, value)
	return nil
}

func newfn[T any]() func() *T {
	return func() *T { return new(T) }
}

func NewValue[T encoding.BinaryValue](batch *Batch, new func() T, key storage.Key) *ValueAs[T] {
	return &ValueAs[T]{batch, key, new}
}

func SubValue[T2, T1 encoding.BinaryValue](v *ValueAs[T1], new func() T2, key ...interface{}) *ValueAs[T2] {
	return NewValue(v.batch, new, v.key.Append(key...))
}

func AccountIndex[T encoding.BinaryValue](b *Batch, u *url.URL, new func() T, key ...interface{}) *ValueAs[T] {
	a := account(u)
	return NewValue(b, new, a.Index(key...))
}

func TransactionIndex[T encoding.BinaryValue](b *Batch, hash []byte, new func() T, key ...interface{}) *ValueAs[T] {
	t := transaction(hash)
	return NewValue(b, new, t.Index(key...))
}
