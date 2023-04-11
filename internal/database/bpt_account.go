// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"bytes"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
)

func (a *Account) VerifyHash(hash []byte) error {
	hasher, err := a.parent.observer.DidChangeAccount(a.parent, a)
	if err != nil {
		return err
	}
	if !bytes.Equal(hash[:], hasher.MerkleHash()) {
		return errors.Conflict.WithFormat("hash does not match")
	}
	return nil
}

// PutBpt writes the record's BPT entry.
func (a *Account) putBpt() error {
	// Ensure the URL state is populated
	_, err := a.getUrl().Get()
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		err = a.getUrl().Put(a.Url())
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	default:
		return errors.UnknownError.Wrap(err)
	}

	hasher, err := a.parent.observer.DidChangeAccount(a.parent, a)
	if err != nil {
		return err
	}

	hash := *(*[32]byte)(hasher.MerkleHash())
	return a.parent.BPT().Insert(a.key.Hash(), hash)
}

// BptReceipt builds a BPT receipt for the account.
func (a *Account) BptReceipt() (*merkle.Receipt, error) {
	if a.IsDirty() {
		return nil, errors.InternalError.With("cannot generate a BPT receipt when there are uncommitted changes")
	}

	receipt, err := a.parent.BPT().GetReceipt(a.key.Hash())
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return receipt, nil
}

// StateReceipt returns a Merkle receipt for the account state in the BPT.
func (a *Account) StateReceipt() (*merkle.Receipt, error) {
	hasher, err := a.parent.observer.DidChangeAccount(a.parent, a)
	if err != nil {
		return nil, err
	}

	rBPT, err := a.BptReceipt()
	if err != nil {
		return nil, err
	}

	rState := hasher.Receipt(0, len(hasher)-1)
	if !bytes.Equal(rState.Anchor, rBPT.Start) {
		return nil, errors.InternalError.With("bpt entry does not match account state")
	}

	receipt, err := rState.Combine(rBPT)
	if err != nil {
		return nil, fmt.Errorf("combine receipt: %w", err)
	}

	return receipt, nil
}
