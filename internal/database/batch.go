package database

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// Batch batches database writes.
type Batch struct {
	done        bool
	writable    bool
	id          int
	nextChildId int
	parent      *Batch
	logger      logging.OptionalLogger
	store       storage.KeyValueTxn
	values      map[storage.Key]record.Record
	bptEntries  map[storage.Key][32]byte
	recordStore record.Store
}

// Begin starts a new batch.
func (d *Database) Begin(writable bool) *Batch {
	d.nextBatchId++

	b := new(Batch)
	b.id = d.nextBatchId
	b.writable = writable
	b.logger.L = d.logger
	b.store = d.store.Begin(writable)
	b.recordStore = record.KvStore{Store: b.store}
	b.values = map[storage.Key]record.Record{}
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
	c.recordStore = batchStore{b}
	c.store = b.store.Begin(c.writable)
	c.values = map[storage.Key]record.Record{}
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

// Commit commits pending writes to the key-value store or the parent batch.
// Attempting to use the Batch after calling Commit or Discard will result in a
// panic.
func (b *Batch) Commit() error {
	if b.done {
		panic("attempted to use a commited or discarded batch")
	}

	b.done = true

	for _, v := range b.values {
		if err := v.Commit(); err != nil {
			return errors.Wrap(errors.StatusUnknownError, err)
		}
	}

	if b.parent != nil {
		for k, v := range b.bptEntries {
			b.parent.bptEntries[k] = v
		}
		if db, ok := b.store.(*storage.DebugBatch); ok {
			db.PretendWrite()
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
	for _, v := range b.values {
		if v.IsDirty() {
			return true
		}
	}
	return false
}
