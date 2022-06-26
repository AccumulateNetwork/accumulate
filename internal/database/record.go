package database

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
)

type RecordStore struct {
	Batch *Batch
}

func (m RecordStore) PutValue(key record.Key, value record.ValueReader) error {
	v, err := value.GetValue()
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}
	m.Batch.putValue(key.Hash(), v)
	return nil
}

func (m *RecordStore) GetValue(key record.Key, value record.ValueWriter) error {
	var didUnmarshal bool
	v, err := m.Batch.getValue(key.Hash(), func(data []byte) (TypedValue, error) {
		didUnmarshal = true
		err := value.LoadBytes(data)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknownError, err)
		}
		return value.GetValue()
	})
	if didUnmarshal || err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	return value.LoadValue(recordValueReader{v}, false)
}

type recordValueReader struct {
	value TypedValue
}

func (r recordValueReader) GetValue() (encoding.BinaryValue, error) {
	return r.value.(encoding.BinaryValue), nil
}
