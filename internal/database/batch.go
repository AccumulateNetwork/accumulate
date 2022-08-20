package database

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// A Beginner can be a Database or a Batch
type Beginner interface {
	Begin(bool) *Batch
	Update(func(*Batch) error) error
	View(func(*Batch) error) error
}

var _ Beginner = (*Database)(nil)
var _ Beginner = (*Batch)(nil)

// Begin starts a new batch.
func (d *Database) Begin(writable bool) *Batch {
	d.nextBatchId++

	b := new(Batch)
	b.id = fmt.Sprint(d.nextBatchId)
	b.writable = writable
	b.logger.L = d.logger
	b.kvstore = d.store.Begin(writable)
	b.store = record.KvStore{Store: b.kvstore}
	b.bptEntries = map[storage.Key][32]byte{}
	return b
}

func (b *Batch) Begin(writable bool) *Batch {
	if writable && !b.writable {
		b.logger.Info("Attempted to create a writable batch from a read-only batch")
	}

	b.nextChildId++

	c := new(Batch)
	c.id = fmt.Sprintf("%s.%d", b.id, b.nextChildId)
	c.writable = b.writable && writable
	c.parent = b
	c.logger = b.logger
	c.store = b
	c.kvstore = b.kvstore.Begin(c.writable)
	c.bptEntries = map[storage.Key][32]byte{}
	return c
}

// DeleteAccountState_TESTONLY is intended for testing purposes only. It deletes an
// account from the database.
func (b *Batch) DeleteAccountState_TESTONLY(url *url.URL) error {
	a := record.Key{"Account", url, "Main"}
	return b.kvstore.Put(a.Hash(), nil)
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

// Commit commits pending writes to the key-value store or the parent batch.
// Attempting to use the Batch after calling Commit or Discard will result in a
// panic.
func (b *Batch) Commit() error {
	if b.done {
		panic(fmt.Sprintf("batch %s: attempted to use a commited or discarded batch", b.id))
	}
	defer func() { b.done = true }()

	err := b.baseCommit()
	if err != nil {
		return errors.StatusUnknownError.Wrap(err)
	}

	if b.parent != nil {
		for k, v := range b.bptEntries {
			b.parent.bptEntries[k] = v
		}
		if db, ok := b.kvstore.(*storage.DebugBatch); ok {
			db.PretendWrite()
		}
	} else {
		err := b.commitBpt()
		if err != nil {
			return errors.StatusUnknownError.Wrap(err)
		}
	}

	return b.kvstore.Commit()
}

// Discard discards pending writes. Attempting to use the Batch after calling
// Discard will result in a panic.
func (b *Batch) Discard() {
	if !b.done && b.writable {
		b.logger.Debug("Discarding a writable batch")
	}
	b.done = true
	b.kvstore.Discard()
}

// Transaction returns an Transaction for the given hash.
func (b *Batch) Transaction(id []byte) *Transaction {
	return b.getTransaction(*(*[32]byte)(id))
}

func (b *Batch) getAccountUrl(key record.Key) (*url.URL, error) {
	v, err := record.NewValue(
		b.logger.L,
		b.store,
		// This must match the key used for the account's main state
		key.Append("Main"),
		"account %[1]v",
		false,
		record.Union(protocol.UnmarshalAccount),
	).Get()
	if err != nil {
		return nil, errors.StatusUnknownError.Wrap(err)
	}
	return v.GetUrl(), nil
}

// AccountByID returns an Account for the given ID.
//
// This is still needed in one place, so the deprecation warning is disabled in
// order to pass static analysis.
//
// Deprecated: Use Account.
func (b *Batch) AccountByID(id []byte) (*Account, error) {
	u, err := b.getAccountUrl(record.Key{"Account", id})
	if err != nil {
		return nil, errors.StatusUnknownError.Wrap(err)
	}
	return b.Account(u), nil
}

// GetValue implements record.Store.
func (b *Batch) GetValue(key record.Key, value record.ValueWriter) error {
	if b.done {
		panic(fmt.Sprintf("batch %s: attempted to use a commited or discarded batch", b.id))
	}

	v, err := resolveValue[record.ValueReader](b, key)
	if err != nil {
		return errors.StatusUnknownError.Wrap(err)
	}

	err = value.LoadValue(v, false)
	return errors.StatusUnknownError.Wrap(err)
}

// PutValue implements record.Store.
func (b *Batch) PutValue(key record.Key, value record.ValueReader) error {
	if b.done {
		panic(fmt.Sprintf("batch %s: attempted to use a commited or discarded batch", b.id))
	}

	v, err := resolveValue[record.ValueWriter](b, key)
	if err != nil {
		return errors.StatusUnknownError.Wrap(err)
	}

	err = v.LoadValue(value, true)
	return errors.StatusUnknownError.Wrap(err)
}

// resolveValue resolves the value for the given key.
func resolveValue[T any](c *Batch, key record.Key) (T, error) {
	var r record.Record = c
	var err error
	for len(key) > 0 {
		r, key, err = r.Resolve(key)
		if err != nil {
			return zero[T](), errors.StatusUnknownError.Wrap(err)
		}
	}

	if s, _, err := r.Resolve(nil); err == nil {
		r = s
	}

	v, ok := r.(T)
	if !ok {
		return zero[T](), errors.StatusInternalError.Format("bad key: %T is not value", r)
	}

	return v, nil
}
