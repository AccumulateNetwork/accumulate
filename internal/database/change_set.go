package database

import (
	"fmt"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

func newChangeSet(id string, writable bool, kvStore storage.KeyValueTxn, store record.Store, logger log.Logger) *ChangeSet {
	c := new(ChangeSet)
	c.logger.Set(logger, "change-set", id)
	c.kvStore = kvStore
	if store == nil {
		c.store = record.KvStore{Store: kvStore}
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

	// Push changes into the store
	err := c.baseCommit()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	// Update the BPT
	err = c.updateBPT()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	// Commit the key-value transaction
	err = c.kvStore.Commit()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	return nil
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

func (c *ChangeSet) GetMinorRootChainAnchor(describe *config.Describe) ([]byte, error) {
	head, err := c.Account(describe.NodeUrl(protocol.Ledger)).RootChain().Minor().Head().Get()
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}
	return head.GetMDRoot(), nil
}

// Import imports values from another database.
func (c *ChangeSet) Import(db interface{ Export() map[storage.Key][]byte }) error {
	err := c.kvStore.PutAll(db.Export())
	return errors.Wrap(errors.StatusUnknown, err)
}
