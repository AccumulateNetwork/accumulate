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
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
)

func newBPT(_ *Batch, logger log.Logger, store record.Store, key record.Key, name, label string) *pmt.Manager {
	_ = name
	return pmt.New(logger, store, key, label)
}

func (b *Batch) putBpt(key, value [32]byte) {
	// Put it all the way up the chain. This is a hack but it works.
	for b := b; b != nil; b = b.parent {
		b.getBPT().InsertKV(key, value)
	}
}

func (b *Batch) VisitAccounts(visit func(*Account) error) error {
	bpt := b.getBPT()

	place := pmt.FirstPossibleBptKey
	const window = 1000 //                                       Process this many BPT entries at a time
	var count int       //                                       Recalculate number of nodes
	for {
		bptVals, next := bpt.Bpt.GetRange(place, int(window)) // Read a thousand values from the BPT
		count += len(bptVals)
		if len(bptVals) == 0 { //                                If there are none left, we break out
			break
		}
		place = next                //                           We will get the next 1000 after the last 1000
		for _, v := range bptVals { //                           For all the key values we got (as many as 1000)
			u, err := b.getAccountUrl(record.Key{storage.Key(v.Key)}) //      Load the Account
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
			err = visit(b.Account(u))
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
		}
	}
	return nil
}

func (b *Batch) ForEachAccount(fn func(account *Account) error) error {
	return b.getBPT().Bpt.ForEach(func(key storage.Key, hash [32]byte) error {
		// Create an Account object
		u, err := b.getAccountUrl(record.Key{key})
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		return fn(b.Account(u))
	})
}

func (b *Batch) SaveAccounts(file io.WriteSeeker, collect func(*Account) ([]byte, error)) error {
	err := b.getBPT().Bpt.SaveSnapshot(file, func(key storage.Key, hash [32]byte) ([]byte, error) {
		// Create an Account object
		u, err := b.getAccountUrl(record.Key{key})
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

// commitBpt commits pending BPT updates.
func (b *Batch) commitBpt() error {
	if !fieldIsDirty(b.bpt) {
		return nil
	}

	bpt := b.getBPT()
	err := bpt.Bpt.Update()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = bpt.Commit()
	return errors.UnknownError.Wrap(err)
}

// BptRoot returns the root of the BPT. BptRoot panics if there are any
// uncommitted BPT changes.
func (b *Batch) BptRoot() []byte {
	bpt := b.getBPT()
	if bpt.IsDirty() {
		panic("attempted to get BPT root with uncommitted changes")
	}
	return bpt.Bpt.RootHash[:]
}

// BptReceipt builds a BPT receipt for the given key.
func (b *Batch) BptReceipt(key storage.Key, value [32]byte) (*merkle.Receipt, error) {
	bpt := b.getBPT()
	if bpt.IsDirty() {
		err := bpt.Commit()
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	receipt := bpt.Bpt.GetReceipt(key)
	if receipt == nil {
		return nil, errors.NotFound.WithFormat("BPT key %v not found", key)
	}

	return receipt, nil
}
