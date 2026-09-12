// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"bytes"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
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

	err = a.parent.BPT().Insert(a.key, hasher.MerkleHash())
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	return a.retainStateReceipt(hasher)
}

// retainStateReceipt keeps the receipt from the account's main state hash to
// its BPT entry, for as long as the BPT retains history.
//
// Without it a historical proof starts at the whole BPT entry —
// H(main, secondary, chains, pending) — which a verifier holding only the
// account state cannot reconstruct, so it has to take the server's word for the
// starting point. With it the historical path starts where [Account.StateReceipt]
// starts, and the proof is checkable offline from the state the query returns.
//
// Nothing is written when retention is off, so a node running the default depth
// of zero stores exactly as many bytes as before.
func (a *Account) retainStateReceipt(hasher hash.Hasher) error {
	height, depth, ok := a.parent.BPT().RetainedWindow()
	if !ok {
		return nil
	}
	if len(hasher) < 2 {
		// The debug observer collapses the components into one hash, so there
		// is no path from the main state to the entry to retain.
		return nil
	}

	blocks, err := a.RetainedStateReceiptBlocks().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load retained state receipt blocks: %w", err)
	}
	if n := len(blocks); n > 0 && blocks[n-1] >= height {
		return nil // Already retained for this block
	}

	err = a.RetainedStateReceipt(height).Put(hasher.Receipt(0, len(hasher)-1))
	if err != nil {
		return errors.UnknownError.WithFormat("retain state receipt: %w", err)
	}

	blocks = append(blocks, height)
	keep, dropped := bpt.PruneHeights(blocks, height, depth)
	for _, d := range dropped {
		// Best effort: an unreferenced receipt leaks bytes, it does not
		// corrupt anything.
		_ = a.RetainedStateReceipt(d).Put(nil)
	}
	err = a.RetainedStateReceiptBlocks().Put(keep)
	return errors.UnknownError.Wrap(err)
}

// BptReceipt builds a BPT receipt for the account.
func (a *Account) BptReceipt() (*merkle.Receipt, error) {
	if a.IsDirty() {
		return nil, errors.InternalError.With("cannot generate a BPT receipt when there are uncommitted changes")
	}

	receipt, err := a.parent.BPT().GetReceipt(a.key)
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
