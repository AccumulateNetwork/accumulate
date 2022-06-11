package database

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type kvStore struct {
	s storage.KeyValueTxn
}

func (s kvStore) get(key recordKey, value encoding.BinaryValue) error {
	b, err := s.s.Get(key.Hash())
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = value.UnmarshalBinary(b)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	return nil
}

func (s kvStore) put(key recordKey, value encoding.BinaryValue) error {
	b, err := value.MarshalBinary()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = s.s.Put(key.Hash(), b)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	return nil
}
