package database

import (
	"encoding"

	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// Value reads and writes a value.
type Value struct {
	batch *Batch
	key   storage.Key
}

// Key returns the value's storage key.
func (v *Value) Key() storage.Key {
	return v.key
}

// Get loads the value.
func (v *Value) Get() ([]byte, error) {
	return v.batch.store.Get(v.key)
}

// GetAs loads the value and unmarshals it into the given value.
func (v *Value) GetAs(u encoding.BinaryUnmarshaler) error {
	data, err := v.Get()
	if err != nil {
		return err
	}

	return u.UnmarshalBinary(data)
}

// Put stores the value.
func (v *Value) Put(data []byte) error {
	return v.batch.store.Put(v.key, data)
}

// PutAs marshals the given value and stores it.
func (v *Value) PutAs(u encoding.BinaryMarshaler) error {
	data, err := u.MarshalBinary()
	if err != nil {
		return err
	}

	return v.Put(data)
}
