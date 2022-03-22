package database

import (
	"encoding"
	"errors"
	"fmt"

	encoding2 "gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// Batch batches database writes.
type Batch struct {
	parent     *Batch
	logger     logging.OptionalLogger
	store      storage.KeyValueTxn
	values     map[storage.Key]cachedValue
	bptEntries map[storage.Key][32]byte
}

// Begin starts a new batch.
func (d *Database) Begin(writable bool) *Batch {
	b := new(Batch)
	b.logger.L = d.logger
	b.store = d.store.Begin(writable)
	b.values = map[storage.Key]cachedValue{}
	b.bptEntries = map[storage.Key][32]byte{}
	return b
}

func (b *Batch) Begin() *Batch {
	c := new(Batch)
	c.parent = b
	c.logger = b.logger
	c.store = b.store.Begin()
	c.values = map[storage.Key]cachedValue{}
	c.bptEntries = map[storage.Key][32]byte{}
	return c
}

type TypedValue interface {
	encoding.BinaryMarshaler
}

type ValueUnmarshalFunc func([]byte) (TypedValue, error)

type cachedValue struct {
	value TypedValue
	dirty bool
}

func (b *Batch) putBpt(key storage.Key, hash [32]byte) {
	b.bptEntries[key] = hash
}

func (b *Batch) getValue(key storage.Key, unmarshal ValueUnmarshalFunc, newValue TypedValue, target interface{}) (err error) {
	// Load the value
	cv, ok := b.values[key]
	var notFound error
	if !ok {
		data, err := b.store.Get(key)
		switch {
		case err == nil:
			// Value is found, unmarshal it
			v, err := unmarshal(data)
			if err != nil {
				return err
			}
			cv = cachedValue{value: v}
			b.values[key] = cv

		case errors.Is(err, storage.ErrNotFound) && newValue != nil:
			// Value is not found, cache the new value
			cv = cachedValue{value: newValue}
			b.values[key] = cv
			notFound = err

		default:
			return err
		}
	}

	err = encoding2.SetPtr(cv.value, target)
	if err != nil {
		return err
	}
	return notFound
}

func (b *Batch) getValuePtr(key storage.Key, value interface {
	TypedValue
	encoding.BinaryUnmarshaler
}, valuePtr interface{}) error {
	return b.getValue(key, func(b []byte) (TypedValue, error) {
		err := value.UnmarshalBinary(b)
		return value, err
	}, value, valuePtr)
}

func (b *Batch) putValue(key storage.Key, value TypedValue) {
	cv, ok := b.values[key]
	if !ok {
		_, err := b.store.Get(key)
		if err == nil {
			b.logger.Info("Overwriting a persisted value", "key", key)
		}
	} else if cv.value != value {
		b.logger.Debug("Overwriting a cached value", "key", key)
	}
	b.values[key] = cachedValue{value: value, dirty: true}
}

func (b *Batch) getAccountStateAs(key storage.Key, newValue protocol.Account, target interface{}) error {
	return b.getValue(key, func(b []byte) (TypedValue, error) {
		return protocol.UnmarshalAccount(b)
	}, newValue, target)
}

func (b *Batch) getAccountState(key storage.Key, newValue protocol.Account) (protocol.Account, error) {
	var v protocol.Account
	err := b.getAccountStateAs(key, newValue, &v)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// CommitBpt updates the Patricia Tree hashes with the values from the updates
// since the last update.
func (b *Batch) CommitBpt() ([]byte, error) {
	bpt := pmt.NewBPTManager(b.store)

	for k, v := range b.bptEntries {
		bpt.InsertKV(k, v)
	}

	err := bpt.Bpt.Update()
	if err != nil {
		return nil, err
	}

	b.bptEntries = nil
	return bpt.Bpt.RootHash[:], nil
}

// Commit commits pending writes to the key-value store or the parent batch.
// Attempting to use the Batch after calling Commit or Discard will result in a
// panic.
func (b *Batch) Commit() error {
	if b.parent != nil {
		for k, v := range b.values {
			if !v.dirty {
				continue
			}
			b.parent.values[k] = v
		}
		for k, v := range b.bptEntries {
			b.parent.bptEntries[k] = v
		}
		return nil
	}

	for k, v := range b.values {
		if !v.dirty {
			continue
		}

		data, err := v.value.MarshalBinary()
		if err != nil {
			return fmt.Errorf("marshal %v: %v", k, err)
		}

		err = b.store.Put(k, data)
		if err != nil {
			return fmt.Errorf("store %v: %v", k, err)
		}
	}
	return b.store.Commit()
}

// Discard discards pending writes. Attempting to use the Batch after calling
// Discard will result in a panic.
func (b *Batch) Discard() {
	b.store.Discard()
}
