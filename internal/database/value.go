package database

import (
	"encoding"

	"github.com/AccumulateNetwork/accumulate/smt/storage"
)

type Value struct {
	batch *Batch
	key   storage.Key
}

func (v *Value) Key() storage.Key {
	return v.key
}

func (v *Value) Get() ([]byte, error) {
	return v.batch.store.Get(v.key)
}

func (v *Value) GetAs(u encoding.BinaryUnmarshaler) error {
	data, err := v.Get()
	if err != nil {
		return err
	}

	return u.UnmarshalBinary(data)
}

func (v *Value) Put(data []byte) {
	v.batch.store.Put(v.key, data)
}

func (v *Value) PutAs(u encoding.BinaryMarshaler) error {
	data, err := u.MarshalBinary()
	if err != nil {
		return err
	}

	v.Put(data)
	return nil
}
