package database

import (
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// Database is an Accumulate database.
type Database struct {
	store  storage.KeyValueStore
	logger logging.OptionalLogger
	nextId uint64
}

// New creates a new database using the given key-value store.
func New(store storage.KeyValueStore, logger log.Logger) *Database {
	d := new(Database)
	d.store = store
	d.logger.Set(logger, "module", "database")

	return d
}

// Begin starts a new nested changeset.
func (d *Database) Begin(writable bool) *ChangeSet {
	d.nextId++
	store := kvStore{d.store.Begin(writable)}
	return newChangeSet(d.nextId, writable, store, d.logger.L)
}

// View runs the function with a read-only transaction.
func (d *Database) View(fn func(cs *ChangeSet) error) error {
	cs := d.Begin(false)
	defer cs.Discard()
	return fn(cs)
}

// Update runs the function with a writable transaction and commits if the
// function succeeds.
func (d *Database) Update(fn func(cs *ChangeSet) error) error {
	cs := d.Begin(true)
	defer cs.Discard()
	err := fn(cs)
	if err != nil {
		return err
	}
	return cs.Commit()
}
