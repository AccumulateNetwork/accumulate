// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"bytes"
	"io"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
)

func newBPT(parent record.Record, logger log.Logger, store record.Store, key *record.Key, name string) *bpt.BPT {
	return bpt.New(parent, logger, store, key)
}

// GetBptRootHash returns the BPT root hash, after applying updates as necessary
// for modified accounts.
func (b *Batch) GetBptRootHash() ([32]byte, error) {
	err := b.UpdateBPT()
	if err != nil {
		return [32]byte{}, errors.UnknownError.Wrap(err)
	}
	return b.BPT().GetRootHash()
}

type AccountIterator struct {
	batch   *Batch
	it      *bpt.Iterator
	entries []bpt.KeyValuePair
	pos     int
	err     error
	current *Account
}

func (it *AccountIterator) Value() *Account { return it.current }

func (it *AccountIterator) Next() bool {
	if it.err != nil {
		return false
	}

	if it.pos >= len(it.entries) {
		it.pos = 0
		if it.it.Next() {
			it.entries = it.it.Value()
		} else {
			it.entries = nil
		}
		if len(it.entries) == 0 {
			return false
		}
	}

	v := it.entries[it.pos]
	it.pos++

	u, err := it.batch.getAccountUrl(v.Key)
	if err != nil {
		it.err = errors.UnknownError.WithFormat("resolve key hash: %w", err)
		return false
	}

	// Create a new account record but don't add it to the map
	it.current = it.batch.newAccount(accountKey{u})
	return true
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
	return bpt.ForEach(b.BPT(), func(key *record.Key, hash []byte) error {
		// Create an Account object
		u, err := b.getAccountUrl(key)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		return fn(b.Account(u), *(*[32]byte)(hash))
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
func (b *Batch) BptReceipt(key *record.Key, value [32]byte) (*merkle.Receipt, error) {
	return b.BPT().GetReceipt(key)
}
