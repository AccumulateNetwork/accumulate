package database

import (
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func newChangeSet(id uint64, writable bool, store recordStore, logger log.Logger) *ChangeSet {
	c := new(ChangeSet)
	c.logger.L = logger
	c.store = store
	c.writable = writable
	c.id = id
	return c
}

// Begin starts a new nested changeset.
func (c *ChangeSet) Begin(writable bool) *ChangeSet {
	if writable && !c.writable {
		c.logger.Info("Attempted to create a writable batch from a read-only batch")
	}

	c.nextId++
	return newChangeSet(c.nextId, c.writable && writable, c, c.logger.L)
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

func (c *ChangeSet) resolveValue(key recordKey) (recordValue, error) {
	var record record = c
	var err error
	for len(key) > 0 {
		record, key, err = record.resolve(key)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknown, err)
		}
	}

	v, ok := record.(recordValue)
	if !ok {
		return nil, errors.New(errors.StatusInternalError, "bad key: not a value")
	}

	return v, nil
}

func (c *ChangeSet) get(key recordKey, value encoding.BinaryValue) error {
	if c.done {
		panic("attempted to use a commited or discarded batch")
	}

	v, err := c.resolveValue(key)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = v.get(value)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	return nil
}

func (c *ChangeSet) put(key recordKey, value encoding.BinaryValue) error {
	if c.done {
		panic("attempted to use a commited or discarded batch")
	}

	v, err := c.resolveValue(key)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = v.put(value)
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
	err := c.commit()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	// If the store is a key-value transaction
	kv, ok := c.store.(kvStore)
	if !ok {
		return nil
	}

	// Update the BPT
	err = c.updateBPT(kv.s)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	// Commit the key-value transaction
	err = kv.s.Commit()
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

	kv, ok := c.store.(*kvStore)
	if !ok {
		return
	}

	kv.s.Discard()
}

func (c *ChangeSet) GetMinorRootChainAnchor(describe *config.Describe) ([]byte, error) {
	return c.Account(describe.NodeUrl(protocol.Ledger)).RootChain().Minor().Anchor()
}
