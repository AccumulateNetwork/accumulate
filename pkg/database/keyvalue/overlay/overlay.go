// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package overlay

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func Open(a, b keyvalue.Beginner) keyvalue.Beginner {
	return &Database{a, b}
}

type Database struct {
	a, b keyvalue.Beginner
}

func (d *Database) Begin(prefix *database.Key, writable bool) keyvalue.ChangeSet {
	a := d.a.Begin(prefix, writable)
	b := d.b.Begin(prefix, false)

	return memory.NewChangeSet(memory.ChangeSetOptions{
		Get:     func(key *record.Key) ([]byte, error) { return get(a, b, key) },
		ForEach: func(fn func(*record.Key, []byte) error) error { return forEach(a, b, fn) },
		Discard: func() { a.Discard(); b.Discard() },
		Commit:  func(m map[[32]byte]memory.Entry) error { return commit(a, b, m) },
	})
}

func get(a, b keyvalue.ChangeSet, key *record.Key) ([]byte, error) {
	// Get from a
	v, err := a.Get(key)
	switch {
	case err == nil:
		return v, nil
	case !errors.Is(err, errors.NotFound):
		return nil, err
	}

	// Get from b
	v, err = b.Get(key)
	switch {
	case err == nil:
		return v, nil
	case !errors.Is(err, errors.NotFound):
		return nil, err
	}

	return nil, (*database.NotFoundError)(key)
}

func forEach(a, b keyvalue.ChangeSet, fn func(*record.Key, []byte) error) error {
	seen := map[[32]byte]bool{}

	err := a.ForEach(func(key *record.Key, value []byte) error {
		seen[key.Hash()] = true
		return fn(key, value)
	})
	if err != nil {
		return err
	}
	return b.ForEach(func(key *record.Key, value []byte) error {
		if seen[key.Hash()] {
			return nil
		}
		return fn(key, value)
	})
}

func commit(a, b keyvalue.ChangeSet, m map[[32]byte]memory.Entry) error {
	for _, entry := range m {
		if !entry.Delete {
			err := a.Put(entry.Key, entry.Value)
			if err != nil {
				return err
			}
			continue
		}

		_, err := b.Get(entry.Key)
		switch {
		case err == nil:
			return errors.NotAllowed.WithFormat("cannot delete %v: it is present in the underlying database", entry.Key)
		case !errors.Is(err, errors.NotFound):
			return err
		}

		err = a.Delete(entry.Key)
		if err != nil {
			return err
		}
	}
	return a.Commit()
}
