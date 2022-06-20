package database

import (
	"encoding"
	"fmt"

	encoding2 "gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// debug is a bit field for enabling debug log messages
//nolint
const debug = 0 |
	// debugGet |
	// debugGetValue |
	// debugPut |
	// debugPutValue |
	// debugCache |
	// debugCacheValue |
	0

const (
	// debugGet logs the key of Batch.getValue
	debugGet = 1 << iota
	// debugGetValue logs the value of Batch.getValue
	debugGetValue
	// debugPut logs the key of Batch.putValue
	debugPut
	// debugPutValue logs the value of Batch.putValue
	debugPutValue
	// debugCache logs the key of Batch.cacheValue
	debugCache
	// debugCacheValue logs the value of Batch.cacheValue
	debugCacheValue
)

// Batch batches database writes.
type Batch struct {
	done        bool
	writable    bool
	dirty       bool
	id          int
	nextChildId int
	parent      *Batch
	logger      logging.OptionalLogger
	store       storage.KeyValueTxn
	values      map[storage.Key]cachedValue
	bptEntries  map[storage.Key][32]byte
}

// Begin starts a new batch.
func (d *Database) Begin(writable bool) *Batch {
	d.nextBatchId++

	b := new(Batch)
	b.id = d.nextBatchId
	b.writable = writable
	b.logger.L = d.logger
	b.store = d.store.Begin(writable)
	b.values = map[storage.Key]cachedValue{}
	// b.values2 = map[string]TypedValue{}
	b.bptEntries = map[storage.Key][32]byte{}
	return b
}

func (b *Batch) Begin(writable bool) *Batch {
	if writable && !b.writable {
		b.logger.Info("Attempted to create a writable batch from a read-only batch")
	}

	b.nextChildId++

	c := new(Batch)
	c.id = b.nextChildId
	c.writable = b.writable && writable
	c.parent = b
	c.logger = b.logger
	c.store = b.store.Begin(c.writable)
	c.values = map[storage.Key]cachedValue{}
	// c.values2 = map[string]TypedValue{}
	c.bptEntries = map[storage.Key][32]byte{}
	return c
}

// DeleteAccountState_TESTONLY is intended for testing purposes only. It deletes an
// account from the database.
func (b *Batch) DeleteAccountState_TESTONLY(url *url.URL) error {
	a := account(url)
	return b.store.Put(a.State(), nil)
}

// View runs the function with a read-only transaction.
func (d *Database) View(fn func(batch *Batch) error) error {
	batch := d.Begin(false)
	defer batch.Discard()
	return fn(batch)
}

// Update runs the function with a writable transaction and commits if the
// function succeeds.
func (d *Database) Update(fn func(batch *Batch) error) error {
	batch := d.Begin(true)
	defer batch.Discard()
	err := fn(batch)
	if err != nil {
		return err
	}
	return batch.Commit()
}

// View runs the function with a read-only transaction.
func (b *Batch) View(fn func(batch *Batch) error) error {
	batch := b.Begin(false)
	defer batch.Discard()
	return fn(batch)
}

// Update runs the function with a writable transaction and commits if the
// function succeeds.
func (b *Batch) Update(fn func(batch *Batch) error) error {
	batch := b.Begin(true)
	defer batch.Discard()
	err := fn(batch)
	if err != nil {
		return err
	}
	return batch.Commit()
}

type TypedValue interface {
	encoding.BinaryMarshaler
	CopyAsInterface() interface{}
}

type TypedValueUnmarshaller interface {
	TypedValue
	encoding.BinaryUnmarshaler
}

type ValueUnmarshalFunc func([]byte) (TypedValue, error)

type cachedValue struct {
	value TypedValue
	dirty bool
}

func (b *Batch) cacheValue(key storage.Key, value TypedValue, dirty bool) {
	// Cache the value, preserve dirtiness
	cv := b.values[key]
	cv.value = value

	switch debug & (debugCache | debugCacheValue) {
	case debugCache | debugCacheValue:
		b.logger.Debug("Cache", "key", key, "value", value, "dirty", logging.WithFormat("%v → %v", cv.dirty, dirty))
	case debugCache:
		b.logger.Debug("Cache", "key", key, "dirty", logging.WithFormat("%v → %v", cv.dirty, dirty))
	case debugCacheValue:
		b.logger.Debug("Cache", "value", value, "dirty", logging.WithFormat("%v → %v", cv.dirty, dirty))
	}

	if dirty {
		b.dirty = true
	}

	if dirty && !cv.dirty {
		cv.dirty = true
	}
	b.values[key] = cv
}

func (b *Batch) getValue(key storage.Key, unmarshal ValueUnmarshalFunc) (v TypedValue, err error) {
	if b.done {
		panic("attempted to use a commited or discarded batch")
	}

	switch debug & (debugGet | debugGetValue) {
	case debugGet | debugGetValue:
		defer func() {
			if err != nil {
				b.logger.Debug("Get", "key", key, "value", err)
			} else {
				b.logger.Debug("Get", "key", key, "value", v)
			}
		}()
	case debugGet:
		b.logger.Debug("Get", "key", key)
	case debugGetValue:
		defer func() {
			if err != nil {
				b.logger.Debug("Get", "error", err)
			} else {
				b.logger.Debug("Get", "value", v)
			}
		}()
	}

	// Check for an existing value
	if cv, ok := b.values[key]; ok {
		return cv.value, nil
	}

	// See if the parent has the value
	if b.parent != nil {
		v, err := b.parent.getValue(key, unmarshal)
		switch {
		case err == nil:
			// Make a copy, otherwise values may leak
			v := v.CopyAsInterface().(TypedValue)
			b.cacheValue(key, v, false)
			return v, nil

		case errors.Is(err, storage.ErrNotFound):
			break

		default:
			return nil, errors.Wrap(errors.StatusUnknown, err)
		}
	}

	data, err := b.store.Get(key)
	switch {
	case err == nil:
		// Value is found, unmarshal it
		v, err := unmarshal(data)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknown, err)
		}

		b.cacheValue(key, v, false)
		return v, nil

	default:
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}
}

func (b *Batch) getValueAs(key storage.Key, unmarshal ValueUnmarshalFunc, newValue TypedValue, target interface{}) (err error) {
	if b.done {
		panic("attempted to use a commited or discarded batch")
	}

	// Load the value
	v, err := b.getValue(key, unmarshal)
	var notFound error
	switch {
	case err == nil:
		// Ok

	case errors.Is(err, storage.ErrNotFound) && newValue != nil:
		// Value is not found, cache the new value
		v = newValue
		b.cacheValue(key, v, false)
		notFound = errors.Wrap(errors.StatusUnknown, err)

	default:
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = encoding2.SetPtr(v, target)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}
	return notFound
}

func (b *Batch) getValuePtr(key storage.Key, value TypedValueUnmarshaller, valuePtr interface{}, addNew bool) error {
	var newValue TypedValue
	if addNew {
		newValue = value
	}
	return b.getValueAs(key, func(b []byte) (TypedValue, error) {
		err := value.UnmarshalBinary(b)
		return value, err
	}, newValue, valuePtr)
}

func (b *Batch) putValue(key storage.Key, value TypedValue) {
	if b.done {
		panic("attempted to use a commited or discarded batch")
	}

	switch debug & (debugPut | debugPutValue) {
	case debugPut | debugPutValue:
		b.logger.Debug("Put", "key", key, "value", value)
	case debugPut:
		b.logger.Debug("Put", "key", key)
	case debugPutValue:
		b.logger.Debug("Put", "value", value)
	}

	cv, ok := b.values[key]
	if !ok {
		_, err := b.store.Get(key)
		if err == nil {
			b.logger.Info("Overwriting a persisted value", "key", key)
		}
	} else if cv.value != value {
		b.logger.Debug("Overwriting a cached value", "key", key)
	}
	b.cacheValue(key, value, true)
}

func (b *Batch) getAccountStateAs(key storage.Key, newValue protocol.Account, target interface{}) error {
	return b.getValueAs(key, func(b []byte) (TypedValue, error) {
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

// Commit commits pending writes to the key-value store or the parent batch.
// Attempting to use the Batch after calling Commit or Discard will result in a
// panic.
func (b *Batch) Commit() error {
	if b.done {
		panic("attempted to use a commited or discarded batch")
	}

	b.done = true

	if b.parent != nil {
		for k, v := range b.values {
			if !v.dirty {
				continue
			}
			b.parent.cacheValue(k, v.value, v.dirty)
		}
		for k, v := range b.bptEntries {
			b.parent.bptEntries[k] = v
		}
		if db, ok := b.store.(*storage.DebugBatch); ok {
			db.PretendWrite()
		}
		return b.store.Commit()
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
	if !b.done && b.writable {
		b.logger.Debug("Discarding a writable batch")
	}
	b.done = true
	b.store.Discard()
}

// Dirty returns true if anything has been changed.
func (b *Batch) Dirty() bool {
	return b.dirty
}
