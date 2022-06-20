package record

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// KvStore is a Store that reads/writes from/to a key-value store.
type KvStore struct {
	Store storage.KeyValueTxn
}

var _ Store = KvStore{}

// GetValue loads the raw value and calls value.LoadBytes.
func (s KvStore) GetValue(key Key, value ValueWriter) error {
	b, err := s.Store.Get(key.Hash())
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = value.LoadBytes(b)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	return nil
}

// PutValue marshals the value and stores it.
func (s KvStore) PutValue(key Key, value ValueReader) error {
	v, err := value.GetValue()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	b, err := v.MarshalBinary()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = s.Store.Put(key.Hash(), b)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	return nil
}