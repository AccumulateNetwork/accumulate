package database

import (
	"fmt"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type kvStore struct {
	Store storage.KeyValueTxn
}

func (kvStore) convertKey(key record.Key) (storage.Key, bool) {
	if len(key) < 2 {
		return storage.Key{}, false
	}
	p1, ok1 := key[0].(string)
	p2, ok2 := key[1].(storage.Key)
	if !ok1 || !ok2 || p1 != "Entry" {
		return storage.Key{}, false
	}
	return p2.Append(key[2:]), true
}

func (s kvStore) GetValue(key record.Key, value record.ValueWriter) error {
	skey, ok := s.convertKey(key)
	if !ok {
		return errors.Format(errors.StatusInternalError, "bad key")
	}

	b, err := s.Store.Get(skey)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = value.LoadBytes(b)
	return errors.Wrap(errors.StatusUnknown, err)
}

func (s kvStore) PutValue(key record.Key, value record.ValueReader) error {
	skey, ok := s.convertKey(key)
	if !ok {
		return errors.Format(errors.StatusInternalError, "bad key")
	}

	v, err := value.GetValue()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	b, err := v.MarshalBinary()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = s.Store.Put(skey, b)
	return errors.Wrap(errors.StatusUnknown, err)
}

func newChangeSet(id string, writable bool, kvs storage.KeyValueTxn, store record.Store, logger log.Logger) *ChangeSet {
	c := new(ChangeSet)
	if logger != nil {
		c.logger.L = logger.With("change-set", id)
	}
	c.kvStore = kvs
	if store == nil {
		c.store = kvStore{Store: kvs}
	} else {
		c.store = store
	}
	c.writable = writable
	c.id = id
	return c
}

// Begin starts a new nested changeset.
func (c *ChangeSet) Begin(writable bool) *ChangeSet {
	if writable && !c.writable {
		c.logger.Info("Attempted to create a writable batch from a read-only batch")
	}
	if !c.writable {
		writable = false
	}

	c.nextId++
	id := fmt.Sprint(c.nextId)
	if c.id != "" {
		id = c.id + "." + id
	}
	return newChangeSet(id, writable, c.kvStore.Begin(writable), c, c.logger.L)
}

// View runs the function with a read-only transaction.
func (c *ChangeSet) View(fn func(cs *ChangeSet) error) error {
	cs := c.Begin(false)
	defer cs.Discard()
	return fn(cs)
}

// Update runs the function with a writable transaction and commits if the
// function succeeds.
func (c *ChangeSet) Update(fn func(cs *ChangeSet) error) error {
	cs := c.Begin(true)
	defer cs.Discard()
	err := fn(cs)
	if err != nil {
		return err
	}
	return cs.Commit()
}

func zero[T any]() (z T) { return }

func resolveValue[T any](c *ChangeSet, key record.Key) (T, error) {
	var r record.Record = c
	var err error
	for len(key) > 0 {
		r, key, err = r.Resolve(key)
		if err != nil {
			return zero[T](), errors.Wrap(errors.StatusUnknown, err)
		}
	}

	if s, _, err := r.Resolve(nil); err == nil {
		r = s
	}

	v, ok := r.(T)
	if !ok {
		return zero[T](), errors.Format(errors.StatusInternalError, "bad key: %T is not value", r)
	}

	return v, nil
}

func (c *ChangeSet) GetValue(key record.Key, value record.ValueWriter) error {
	if c.done {
		panic("attempted to use a commited or discarded batch")
	}

	v, err := resolveValue[record.ValueReader](c, key)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = value.LoadValue(v, false)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	return nil
}

func (c *ChangeSet) PutValue(key record.Key, value record.ValueReader) error {
	if c.done {
		panic("attempted to use a commited or discarded batch")
	}

	v, err := resolveValue[record.ValueWriter](c, key)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = v.LoadValue(value, true)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	return nil
}

func (c *ChangeSet) Commit() error {
	if c.done {
		panic("attempted to use a commited or discarded batch")
	}
	c.done = true
	c.logger.Debug("Commit change set")

	return c.baseCommit()
}

// Discard discards pending writes. Attempting to use the Batch after calling
// Discard will result in a panic.
func (c *ChangeSet) Discard() {
	if !c.done {
		if c.writable {
			c.logger.Debug("Discarding a writable change set")
		} else {
			c.logger.Debug("Discard change set")
		}
	}
	c.done = true

	c.kvStore.Discard()
}

// Import imports values from another database.
func (c *ChangeSet) Import(db interface{ Export() map[storage.Key][]byte }) error {
	err := c.kvStore.PutAll(db.Export())
	return errors.Wrap(errors.StatusUnknown, err)
}
