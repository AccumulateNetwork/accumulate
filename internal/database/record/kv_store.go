package record

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
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
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	// Tests may 'delete' an account by setting its value to nil
	if len(b) == 0 {
		return errors.NotFound("%v not found", key)
	}

	err = value.LoadBytes(b)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	return nil
}

// PutValue marshals the value and stores it.
func (s KvStore) PutValue(key Key, value ValueReader) error {
	v, err := value.GetValue()
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	b, err := v.MarshalBinary()
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	err = s.Store.Put(key.Hash(), b)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	return nil
}
