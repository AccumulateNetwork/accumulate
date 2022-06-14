package database

import (
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

func newChangeSet(id uint64, writable bool, kvStore storage.KeyValueTxn, store record.Store, logger log.Logger) *ChangeSet {
	c := new(ChangeSet)
	c.logger.L = logger
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
	return newChangeSet(c.nextId, writable, c.kvStore.Begin(writable), c, c.logger.L)
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

func (c *ChangeSet) resolveValue(key record.Key) (record.ValueReadWriter, error) {
	var r record.Record = c
	var err error
	for len(key) > 0 {
		r, key, err = r.Resolve(key)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknown, err)
		}
	}

	if s, _, err := r.Resolve(nil); err == nil {
		r = s
	}

	v, ok := r.(record.ValueReadWriter)
	if !ok {
		return nil, errors.Format(errors.StatusInternalError, "bad key: %T is not value", r)
	}

	return v, nil
}

func (c *ChangeSet) LoadValue(key record.Key, value record.ValueReadWriter) error {
	if c.done {
		panic("attempted to use a commited or discarded batch")
	}

	v, err := c.resolveValue(key)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = value.ReadFrom(v)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	return nil
}

func (c *ChangeSet) StoreValue(key record.Key, value record.ValueReadWriter) error {
	if c.done {
		panic("attempted to use a commited or discarded batch")
	}

	v, err := c.resolveValue(key)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = v.ReadFrom(value)
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
	if !c.done && c.writable {
		c.logger.Debug("Discarding a writable batch")
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
