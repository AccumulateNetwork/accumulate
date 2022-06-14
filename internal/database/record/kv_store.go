package record

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type KvStore struct {
	Store storage.KeyValueTxn
}

var _ Store = KvStore{}

func (s KvStore) LoadValue(key Key, value ValueReadWriter) error {
	b, err := s.Store.Get(key.Hash())
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = value.LoadFrom(b)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	return nil
}

func (s KvStore) StoreValue(key Key, value ValueReadWriter) error {
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
