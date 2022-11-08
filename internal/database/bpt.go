// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"bytes"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
)

func (b *Batch) VisitAccounts(visit func(*Account) error) error {
	bpt := pmt.NewBPTManager(b.kvstore)

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
				return errors.Wrap(errors.StatusUnknownError, err)
			}
			err = visit(b.Account(u))
			if err != nil {
				return errors.Wrap(errors.StatusUnknownError, err)
			}
		}
	}
	return nil
}

func (b *Batch) SaveAccounts(file io.WriteSeeker, collect func(*Account) ([]byte, error)) error {
	bpt := pmt.NewBPTManager(b.kvstore)
	err := bpt.Bpt.SaveSnapshot(file, func(key storage.Key, hash [32]byte) ([]byte, error) {
		// Create an Account object
		u, err := b.getAccountUrl(record.Key{key})
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknownError, err)
		}
		account := b.Account(u)

		// Check the hash
		hasher, err := account.hashState()
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "hash %v: %w", u, err)
		}

		if !bytes.Equal(hash[:], hasher.MerkleHash()) {
			return nil, errors.Format(errors.StatusConflict, "hash does not match for %v", u)
		}

		state, err := collect(account)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "collect %v: %w", u, err)
		}
		return state, nil
	})
	return errors.Wrap(errors.StatusUnknownError, err)
}

// putBpt adds an entry to the list of pending BPT updates.
func (b *Batch) putBpt(key storage.Key, hash [32]byte) {
	if b.done {
		panic("attempted to use a commited or discarded batch")
	}
	if b.bptEntries == nil {
		panic("attempted to update the BPT after committing the BPT")
	}

	b.bptEntries[key] = hash
}

// commitBpt commits pending BPT updates.
func (b *Batch) commitBpt() error {
	if len(b.bptEntries) == 0 {
		return nil
	}

	bpt := pmt.NewBPTManager(b.kvstore)

	for k, v := range b.bptEntries {
		bpt.InsertKV(k, v)
	}

	err := bpt.Bpt.Update()
	if err != nil {
		return err
	}

	b.bptEntries = nil
	return nil
}

// BptRoot returns the root of the BPT. BptRoot panics if there are any
// uncommitted BPT changes.
func (b *Batch) BptRoot() []byte {
	if len(b.bptEntries) > 0 {
		panic("attempted to get BPT root with uncommitted changes")
	}
	bpt := pmt.NewBPTManager(b.kvstore)
	return bpt.Bpt.RootHash[:]
}

// BptReceipt builds a BPT receipt for the given key.
func (b *Batch) BptReceipt(key storage.Key, value [32]byte) (*managed.Receipt, error) {
	if len(b.bptEntries) > 0 {
		return nil, errors.New(errors.StatusInternalError, "cannot generate a BPT receipt when there are uncommitted BPT entries")
	}

	bpt := pmt.NewBPTManager(b.kvstore)
	receipt := bpt.Bpt.GetReceipt(key)
	if receipt == nil {
		return nil, errors.NotFound("BPT key %v not found", key)
	}

	return receipt, nil
}
