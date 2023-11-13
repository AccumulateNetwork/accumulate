// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package badger

import (
	"sync"

	"github.com/dgraph-io/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

type ChangeSet struct {
	db       *Database
	badger   *badger.Txn
	writable bool
	prefix   *record.Key
}

// Begin begins a nested change set.
func (c *ChangeSet) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	var commit memory.CommitFunc
	if writable {
		if !c.writable {
			panic("attempted to create a writable change set from a read-only one")
		}
		commit = c.putAll
	}
	return memory.NewChangeSet(prefix, c.Get, commit)
}

// lock calls (*Database).lock and _also_ discards the change set if the lock
// fails. Why? No clue; that's what the previous version did so I preserved the
// logic in case it matters. It probably has something to do with concurrency
// hell.
func (c *ChangeSet) lock() (sync.Locker, error) {
	l, err := c.db.lock(false)
	if err == nil {
		return l, nil
	}

	c.Discard() // Is this a good idea?
	return nil, err
}

func (c *ChangeSet) Put(key *record.Key, value []byte) error {
	if l, err := c.lock(); err != nil {
		return err
	} else {
		defer l.Unlock()
	}

	if !c.writable {
		return errors.NotAllowed.With("change set is not writable")
	}
	kh := c.prefix.AppendKey(key).Hash()
	return c.badger.Set(kh[:], value)
}

func (c *ChangeSet) Delete(key *record.Key) error {
	if l, err := c.lock(); err != nil {
		return err
	} else {
		defer l.Unlock()
	}

	if !c.writable {
		return errors.NotAllowed.With("change set is not writable")
	}
	kh := c.prefix.AppendKey(key).Hash()
	return c.badger.Delete(kh[:])
}

func (c *ChangeSet) putAll(entries map[[32]byte]memory.Entry) error {
	if l, err := c.lock(); err != nil {
		return err
	} else {
		defer l.Unlock()
	}

	for _, e := range entries {
		kh := c.prefix.AppendKey(e.Key).Hash()

		var err error
		if e.Delete {
			err = c.badger.Delete(kh[:])
		} else {
			err = c.badger.Set(kh[:], e.Value)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *ChangeSet) Get(key *record.Key) ([]byte, error) {
	if l, err := c.lock(); err != nil {
		return nil, err
	} else {
		defer l.Unlock()
	}

	kh := c.prefix.AppendKey(key).Hash()
	item, err := c.badger.Get(kh[:])
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, badger.ErrKeyNotFound):
		return nil, (*database.NotFoundError)(key)
	default:
		return nil, err
	}

	// If we didn't find the value, return ErrNotFound
	v, err := item.ValueCopy(nil)
	switch {
	case err == nil:
		return v, nil
	case errors.Is(err, badger.ErrKeyNotFound):
		return nil, (*database.NotFoundError)(key)
	default:
		return nil, errors.UnknownError.WithFormat("get %v: %w", key, err)
	}
}

func (c *ChangeSet) Commit() error {
	if l, err := c.lock(); err != nil {
		return err
	} else {
		defer l.Unlock()
	}

	return c.badger.Commit()
}

func (c *ChangeSet) Discard() {
	c.badger.Discard()
}
