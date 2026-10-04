// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"io"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/leveldb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const markPower = 8

// Database is an Accumulate database.
type Database struct {
	store       keyvalue.Beginner
	logger      logging.Logger
	nextBatchId int64
	observer    Observer

	// gates is the section gate this database and its views share
	// (sections.go; #4485).
	gates *sectionGates

	// sinks is where batches report their BPT commits (bpt_trail.go).
	sinks *bptSinks
}

// New creates a new database using the given key-value store.
func New(store keyvalue.Beginner, logger logging.Logger) *Database {
	d := new(Database)
	d.store = store
	d.observer = NewDatabaseObserver()
	d.gates = new(sectionGates)
	d.sinks = new(bptSinks)

	if logger != nil {
		d.logger = logger.With("module", "database")
	}

	return d
}

// Deep returns a view of this database whose batches read the store's
// whole history, not just the recent window it answers protocol reads
// from.
//
// BlockchainDB answers a permanent read from the last N to 2N blocks
// and calls anything older absent: probing history per segment on
// every miss was 23% of a validator's CPU and grew with the chain. The
// data is still there. A reader that knowingly looks back -- the API
// serving an explorer, healing re-proving an old range -- takes this
// once, at construction, and every batch it begins reaches history
// without any of its call sites changing. The executor keeps the
// windowed database, so the cost stays with the callers that need it.
//
// On a store with no window this returns an equivalent database: their
// batches already read everything.
func (d *Database) Deep() *Database {
	return d.view(keyvalue.Deep(d.store))
}

// Unisolated returns a view of this database whose batches read the
// latest committed state and pin no version.
//
// BlockchainDB keeps isolation for a batch by remembering, for every
// commit made while the batch is open, what each rewritten key held
// before -- a store read per dynamic entry per commit. A reader that
// wants the latest state gains nothing from that: CheckTx validates a
// submission against whatever has been committed so far, and taking an
// ordinary batch for it meant a reader was almost always open while a
// block committed. On a store that keeps isolation for free this returns
// an equivalent database.
func (d *Database) Unisolated() *Database {
	return d.view(keyvalue.Unisolated(d.store))
}

// view is this database over store. It is not a copy of the struct: a batch
// begun on another goroutine counts nextBatchId atomically, and a plain copy
// reads it without (#4472: the join's backfill begins batches beside the
// executor's CheckTx).
func (d *Database) view(store keyvalue.Beginner) *Database {
	return &Database{
		store:       store,
		logger:      d.logger,
		nextBatchId: atomic.LoadInt64(&d.nextBatchId),
		observer:    d.observer,
		gates:       d.gates,
		sinks:       d.sinks,
	}
}

func OpenInMemory(logger logging.Logger) *Database {
	store := memory.New(nil)
	return New(store, logger)
}

func OpenBadger(filepath string, logger logging.Logger) (*Database, error) {
	store, err := badger.New(filepath)
	if err != nil {
		return nil, err
	}
	return New(store, logger), nil
}

func OpenLevelDB(filepath string, logger logging.Logger) (*Database, error) {
	store, err := leveldb.Open(filepath)
	if err != nil {
		return nil, err
	}
	return New(store, logger), nil
}

// Store returns the underlying key-value store. Store may return an error in
// the future.
func (d *Database) Store() (keyvalue.Beginner, error) {
	return d.store, nil
}

// SetObserver sets the database observer.
//
// Deprecated: The production observer is now the default. This method should
// only be used for testing with specialized observers.
func (d *Database) SetObserver(observer Observer) {
	if observer == nil {
		panic("SetObserver called with nil")
	}
	d.observer = observer
}

// Close closes the database and the key-value store.
func (d *Database) Close() error {
	if c, ok := d.store.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func (b *Batch) GetMinorRootChainAnchor(describe *config.Describe) ([]byte, error) {
	ledger := b.Account(describe.NodeUrl(protocol.Ledger))
	chain, err := ledger.RootChain().Get()
	if err != nil {
		return nil, err
	}
	return chain.Anchor(), nil
}

type Observer interface {
	DidChangeAccount(batch *Batch, account *Account) (hash.Hasher, error)
}
