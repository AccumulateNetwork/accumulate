// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"bytes"
	"io"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
)

func newBPT(parent record.Record, logger log.Logger, store record.Store, key *record.Key, name, label string) bpt.BPT {
	return bpt.New(parent, logger, store, key, label)
}

type AccountIterator struct {
	batch   *Batch
	it      bpt.Iterator
	entries []bpt.KeyValuePair
	pos     int
	err     error
}

func (it *AccountIterator) Next() (*Account, bool) {
	if it.err != nil {
		return nil, false
	}

	if it.pos >= len(it.entries) {
		var ok bool
		it.pos = 0
		it.entries, ok = it.it.Next()
		if !ok || len(it.entries) == 0 {
			return nil, false
		}
	}

	v := it.entries[it.pos]
	it.pos++

	u, err := it.batch.getAccountUrl(record.NewKey(storage.Key(v.Key)))
	if err != nil {
		it.err = errors.UnknownError.WithFormat("resolve key hash: %w", err)
		return nil, false
	}

	// Create a new account record but don't add it to the map
	return it.batch.newAccount(accountKey{u}), true
}

func (it *AccountIterator) Err() error {
	if it.err != nil {
		return it.err
	}
	return it.it.Err()
}

func (b *Batch) IterateAccounts() *AccountIterator {
	return &AccountIterator{
		batch: b,
		it:    b.BPT().Iterate(1000),
	}
}

func (b *Batch) ForEachAccount(fn func(account *Account, hash [32]byte) error) error {
	return bpt.ForEach(b.BPT(), func(key storage.Key, hash [32]byte) error {
		// Create an Account object
		u, err := b.getAccountUrl(record.NewKey(key))
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		return fn(b.Account(u), hash)
	})
}

func (b *Batch) SaveAccounts(file io.WriteSeeker, collect func(*Account) ([]byte, error)) error {
	err := bpt.SaveSnapshotV1(b.BPT(), file, func(key storage.Key, hash [32]byte) ([]byte, error) {
		// Create an Account object
		u, err := b.getAccountUrl(record.NewKey(key))
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		account := b.Account(u)

		// Check the hash
		if _, ok := b.observer.(unsetObserver); !ok {
			hasher, err := b.observer.DidChangeAccount(b, account)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("hash %v: %w", u, err)
			}

			if !bytes.Equal(hash[:], hasher.MerkleHash()) {
				return nil, errors.Conflict.WithFormat("hash does not match for %v", u)
			}
		}

		state, err := collect(account)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("collect %v: %w", u, err)
		}
		return state, nil
	})
	return errors.UnknownError.Wrap(err)
}

// BptReceipt builds a BPT receipt for the given key.
func (b *Batch) BptReceipt(key storage.Key, value [32]byte) (*merkle.Receipt, error) {
	return b.BPT().GetReceipt(key)
}
