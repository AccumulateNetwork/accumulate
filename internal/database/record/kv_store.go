package record

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type KvStore struct {
	Store storage.KeyValueTxn
}

func (s KvStore) GetRaw(key Key, value encoding.BinaryValue) error {
	b, err := s.Store.Get(key.Hash())
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = value.UnmarshalBinary(b)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	return nil
}

func (s KvStore) PutRaw(key Key, value encoding.BinaryValue) error {
	b, err := value.MarshalBinary()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = s.Store.Put(key.Hash(), b)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	return nil
}
