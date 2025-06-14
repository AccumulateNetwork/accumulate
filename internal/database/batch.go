// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

type Viewer interface {
	View(func(batch *Batch) error) error
}

type Updater interface {
	Viewer
	Update(func(batch *Batch) error) error
}

// A Beginner can be a Database or a Batch
type Beginner interface {
	Updater
	Begin(bool) *Batch
	SetObserver(Observer)
}

var _ Beginner = (*Database)(nil)
var _ Beginner = (*Batch)(nil)

// SetObserver sets the database observer.
func (b *Batch) SetObserver(observer Observer) {
	if observer == nil {
		observer = unsetObserver{}
	}
	b.observer = observer
}

// Begin starts a new batch.
func (d *Database) Begin(writable bool) *Batch {
	id := atomic.AddInt64(&d.nextBatchId, 1)

	b := NewBatch(fmt.Sprint(id), d.store.Begin(nil, writable), writable, d.logger)
	b.observer = d.observer
	return b
}

func NewBatch(id string, store keyvalue.Store, writable bool, logger log.Logger) *Batch {
	b := new(Batch)
	b.id = id
	b.writable = writable
	b.logger.Set(logger)
	b.store = keyvalue.RecordStore{Store: store}
	return b
}

func (b *Batch) Begin(writable bool) *Batch {
	if writable && !b.writable {
		b.logger.Info("Attempted to create a writable batch from a read-only batch")
	}

	b.nextChildId++

	c := new(Batch)
	c.id = fmt.Sprintf("%s.%d", b.id, b.nextChildId)
	c.observer = b.observer
	c.writable = b.writable && writable
	c.parent = b
	c.logger = b.logger
	c.store = values.RecordStore{Record: b}
	return c
}

// DeleteAccountState_TESTONLY is intended for testing purposes only. It deletes
// an account from the database. It will panic if the batch's store is not a
// key-value store.
func (b *Batch) DeleteAccountState_TESTONLY(url *url.URL) error {
	a := record.NewKey("Account", url, "Main")
	return b.kvs().Put(a, nil)
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
	err = batch.UpdateBPT()
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
		panic(fmt.Sprintf("batch %s: attempted to use a committed or discarded batch", b.id))
	}
	defer func() { b.done = true }()

	err := b.baseCommit()
	if err != nil {
		// An error during commit indicates a serious bug and likely will result
		// in a corrupted database (partial write) so it's time to panic
		panic(errors.UnknownError.Wrap(err))
	}

	// Committing may have changed the BPT, so commit it
	values.Commit(&err, b.bpt)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	if b.parent != nil {
		return nil
	}

	return b.kvs().Commit()
}

// Discard discards pending writes. Attempting to use the Batch after calling
// Discard will result in a panic.
func (b *Batch) Discard() {
	b.done = true

	// If the parent is nil, the store _must_ be a key-value store
	if b.parent == nil {
		b.kvs().Discard()
	}
}

func (b *Batch) kvs() keyvalue.ChangeSet {
	return b.store.(interface{ Unwrap() keyvalue.Store }).Unwrap().(keyvalue.ChangeSet)
}

// Transaction returns an Transaction for the given hash.
func (b *Batch) Transaction(id []byte) *Transaction {
	return b.getTransaction(*(*[32]byte)(id))
}

func (b *Batch) Transaction2(id [32]byte) *Transaction {
	return b.getTransaction(id)
}

func (b *Batch) getAccountUrl(key *record.Key) (*url.URL, error) {
	if key.Len() >= 2 && key.Get(0) == "Account" {
		return key.Get(1).(*url.URL), nil
	}

	v, err := values.NewValue(
		b.logger.L,
		b.store,
		// This must match the key used for the account's Url state
		key.Append("Url"),
		false,
		values.Wrapped(values.UrlWrapper),
	).Get()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return v, nil
}

// AI: resolveAccountKey attempts to resolve a database key that may begin with
// AI: a KeyHash. If the key does not start with a KeyHash, it is returned
// AI: unchanged. If it does, the function looks up the corresponding account
// AI: URL using getAccountUrl. If successful, it constructs a new key where
// AI: the first component is replaced by "Account" and the resolved URL,
// AI: followed by the remaining components. If resolution fails, the original
// AI: key is returned and an error is logged.
// AI:
// AI: Example 1 (short URL):
// AI:   key = record.NewKey("Account", "RedWagon.acme")
// AI:   // Used directly; no hash needed.
// AI:
// AI: Example 2 (long URL):
// AI:   key = record.NewKey(KeyHash(0x1234...), "Url")
// AI:   // The long URL is hashed to produce KeyHash. The resolver attempts to
// AI:   // recover the original URL from the hash using getAccountUrl.
// AI:   // If found, the key is rewritten as record.NewKey("Account", url).
// AI:   // If not found, the hash-based key remains.
func (b *Batch) resolveAccountKey(key *record.Key) *record.Key {
	// Check if the key is short or does not start with a KeyHash
	if key.Len() < 1 {
		return key
	}

	// Detect if the key starts with a KeyHash
	kh, ok := key.Get(0).(record.KeyHash)
	if !ok {
		return key
	}

	u, err := b.getAccountUrl(key.SliceJ(1))
	if err != nil {
		slog.Error("Unable to resolve account in the BPT", "key-hash", logging.AsHex(kh), "error", err)
		return key
	}

	return record.NewKey("Account", u).AppendKey(key.SliceI(1))
}

// AI: keyIsAccountUrl checks if a key is of the form [KeyHash, "Url"], which
// AI: is used to identify account URL keys in the database.
func keyIsAccountUrl(key *record.Key) bool {
	if key.Len() != 2 {
		return false
	}

	_, ok0 := key.Get(0).(record.KeyHash)
	s, ok1 := key.Get(1).(string)
	return ok0 && ok1 && s == "Url"
}

func (b *Batch) Resolve(key *record.Key) (database.Record, *record.Key, error) {
	// Calling getAccountUrl leads to calling Resolve with `<key hash>.Url`.
	// Since the default resolver does not know how to handle that, detect that
	// pattern and handle it here.

	if !keyIsAccountUrl(key) {
		return b.baseResolve(key)
	}

	v := values.NewValue(
		b.logger.L,
		b.store,
		key,
		false,
		values.Wrapped(values.UrlWrapper),
	)
	return v, key.SliceI(2), nil
}

// UpdatedAccounts returns every account updated in this database batch.
func (b *Batch) UpdatedAccounts() []*Account {
	accounts := make([]*Account, 0, len(b.account))
	for _, a := range b.account {
		if a.IsDirty() {
			accounts = append(accounts, a)
		}
	}
	return accounts
}
