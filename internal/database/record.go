package database

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

func (b *Batch) resolve(key record.Key, requester record.ShimValue) (record.Record, record.Key, error) {
	if len(key) == 0 {
		return nil, nil, errors.New(errors.StatusInternalError, "bad key for batch: empty")
	}

	skey, ok := key[0].(storage.Key)
	if !ok {
		return nil, nil, errors.Format(errors.StatusInternalError, "bad key for value: expected %T, got %T", storage.Key{}, key[0])
	}

	if len(key) == 1 {
		if requester == nil {
			return nil, nil, errors.New(errors.StatusInternalError, "bad key for batch: expected value but requester is nil")
		}
		v, err := getOrCreateRecord(b, skey, func() record.Record { return requester.NewCopy(b.recordStore) })
		return v, nil, err
	}

	if len(key) >= 3 {
		v, err := getOrCreateRecord(b, key[:3].Hash(), func() *managed.Chain {
			return managed.NewChain(b.logger.L, b.recordStore, key[:3], markPower, "account %[1]s chain %[3]s")
		})
		return v, key[3:], err
	}

	return nil, nil, errors.New(errors.StatusInternalError, "bad key for batch")
}

type batchStore struct {
	*Batch
}

func (b batchStore) GetValue(key record.Key, value record.ValueWriter) error {
	if b.done {
		panic("attempted to use a commited or discarded batch")
	}

	v, err := b.resolveValue(key, value)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	err = value.LoadValue(v, false)
	return errors.Wrap(errors.StatusUnknownError, err)
}

func (b batchStore) PutValue(key record.Key, value record.ValueReader) error {
	if b.done {
		panic("attempted to use a commited or discarded batch")
	}

	v, err := b.resolveValue(key, value)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	err = v.LoadValue(value, true)
	return errors.Wrap(errors.StatusUnknownError, err)
}

func (b batchStore) resolveValue(key record.Key, requester interface{}) (record.ShimValue, error) {
	rshim, _ := requester.(record.ShimValue)
	v, key, err := b.resolve(key, rshim)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	for len(key) > 0 {
		v, key, err = v.Resolve(key)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknownError, err)
		}
	}

	if u, _, err := v.Resolve(nil); err == nil {
		v = u
	}

	u, ok := v.(record.ShimValue)
	if !ok {
		return nil, errors.Format(errors.StatusInternalError, "bad key: %T is not a value", v)
	}

	return u, nil
}

func getOrCreateRecord[T record.Record](batch *Batch, key storage.Key, create func() T) (T, error) {
	if v, ok := batch.values[key]; ok {
		u, ok := v.(T)
		if ok {
			return u, nil
		}
		return u, errors.Format(errors.StatusInternalError, "expected %T, got %T", u, v)
	}

	v := create()
	batch.values[key] = v
	return v, nil
}

func getOrCreateValue[T any](batch *Batch, key storage.Key, allowMissing bool, value record.EncodableValue[T]) Value[T] {
	v, err := getOrCreateRecord(batch, key, func() *record.Value[T] {
		return record.NewValue(batch.logger, batch.recordStore, record.Key{key}, key.String(), allowMissing, value)
	})
	if err != nil {
		return errorValue[T]{errors.Wrap(errors.StatusUnknownError, err)}
	}
	return v
}

type Value[T any] interface {
	Get() (T, error)
	GetAs(target interface{}) error
	Put(T) error
}

func AccountIndex[T any](b *Batch, u *url.URL, allowMissing bool, value record.EncodableValue[T], key ...interface{}) Value[T] {
	a := account(u)
	return getOrCreateValue(b, a.Index(key...), allowMissing, value)
}

func TransactionIndex[T any](b *Batch, hash []byte, allowMissing bool, value record.EncodableValue[T], key ...interface{}) Value[T] {
	t := transaction(hash)
	return getOrCreateValue(b, t.Index(key...), allowMissing, value)
}

type errorValue[T any] struct {
	error
}

func (v errorValue[T]) Get() (u T, _ error)            { return u, v.error }
func (v errorValue[T]) GetAs(target interface{}) error { return v.error }
func (v errorValue[T]) Put(T) error                    { return v.error }
