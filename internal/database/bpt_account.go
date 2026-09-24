// Copyright 2026 The Accumulate Authors
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
	// The state tree holds a leaf only for an account that exists, and an
	// account exists when it has main state (executor spec, invariant 13;
	// #4437). A write to a missing account's bookkeeping alone — a
	// transaction's votes, payments or signatures recorded against a
	// principal that turns out not to exist — marks it dirty, and without
	// this it would get a leaf hashing to nothing.
	_, err := a.Main().Get()
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		return nil
	default:
		return errors.UnknownError.Wrap(err)
	}

	// Ensure the URL state is populated
	_, err = a.getUrl().Get()
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
// Nothing is written when retention is off, so a node running a depth of zero
// stores exactly as many bytes as before. Retention costs, and it is worth
// stating plainly:
//
//	retained bytes ~= dirty accounts per block x depth x 146
//
// 146 is the marshalled receipt, not the 64 bytes of new information in it: the
// record also carries Start (the main state hash, which the verifier supplies)
// and Anchor (the BPT entry, which BPT history already holds for that block),
// plus framing. Storing the two sibling hashes alone would realise the saving
// over retaining the three state components, which cost 96. That is an
// optimisation and not a defect — the receipt composes directly with Combine,
// which the bare siblings would not.
//
// The body and the rest of the leaf are retained beside it, and they cost far
// more: the body is the account's marshalled state, and the leaf holds every
// chain's head (up to one hash per bit of the chain's count), the directory,
// the pending sets and, for the ledgers, the delivery queues. That is the price
// of serving the whole leaf as of a block rather than only proving it (#4361).
//
// # A dormant account keeps more than it needs
//
// Pruning runs here, so it runs only when an account is written. An account
// that stops changing keeps every receipt it had when it went quiet. It is
// bounded (it stops growing when the account does), it never exceeds
// depth x 146 bytes for one account, and it has no effect on correctness:
// retainedStateReceipt always takes the newest receipt at or before the block,
// so the unreachable ones are never read.
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
	// THE LAST WRITE IN A BLOCK WINS. putBpt runs more than once for an
	// account in one block (transactionStatus.Put, then UpdateBPT), and the
	// entry the block ends with is the last one written. Keeping the first
	// would retain a leaf the block's root does not contain.
	n := len(blocks)
	if n > 0 && blocks[n-1] > height {
		return nil
	}
	rewrite := n > 0 && blocks[n-1] == height

	err = a.RetainedStateReceipt(height).Put(hasher.Receipt(0, len(hasher)-1))
	if err != nil {
		return errors.UnknownError.WithFormat("retain state receipt: %w", err)
	}

	// And the body that receipt starts at. A historical receipt served beside
	// the CURRENT body proves nothing a caller can check: the body it was
	// handed does not hash to the receipt's start, so a pulling node refuses
	// it and a trusting one keeps the wrong state (#4361, executor.md "Sync"
	// §2, "A body served with a proof is the stored body, byte for byte").
	state, err := a.Main().Get()
	switch {
	case err == nil:
		// Kept as the marshalled form, which is what the hasher hashed
		// (observer_prod.go, hashValue): a caller recomputes the receipt's
		// start from the bytes it was served, with no re-marshalling in
		// between to differ.
		encoded, err := state.MarshalBinary()
		if err != nil {
			return errors.UnknownError.WithFormat("marshal main state: %w", err)
		}
		err = a.RetainedMainState(height).Put(encoded)
		if err != nil {
			return errors.UnknownError.WithFormat("retain main state: %w", err)
		}
	case errors.Is(err, errors.NotFound):
		// An account with chains and no main state: there is no body to
		// retain and nothing will be served for it.
		if rewrite {
			_ = a.RetainedMainState(height).Put(nil)
		}
	default:
		return errors.UnknownError.WithFormat("load main state: %w", err)
	}

	// And the rest of the leaf, so the whole entry can be served as of the
	// block and not only the body (#4361).
	leaf, err := a.LeafState()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	err = a.RetainedLeaf(height).Put(leaf)
	if err != nil {
		return errors.UnknownError.WithFormat("retain leaf: %w", err)
	}

	if rewrite {
		return nil
	}
	blocks = append(blocks, height)
	keep, dropped := bpt.PruneHeights(blocks, height, depth)
	for _, d := range dropped {
		// Best effort: an unreferenced receipt or body leaks bytes, it does
		// not corrupt anything.
		_ = a.RetainedStateReceipt(d).Put(nil)
		_ = a.RetainedMainState(d).Put(nil)
		_ = a.RetainedLeaf(d).Put(nil)
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

// StateTreeReceipt returns a Merkle receipt proving the account's state into
// its BPT leaf value. It is the first half of StateReceipt, without the BPT
// path, and it is what a node pulling state needs (executor.md, "Sync"): the
// peer's receipt carries the path from the leaf to the anchored root, and this
// says the state the node just pulled is what sits at the foot of it.
//
// Unlike StateReceipt it does not read the BPT, so it works on an uncommitted
// batch — the pull verifies before it commits.
func (a *Account) StateTreeReceipt() (*merkle.Receipt, error) {
	hasher, err := a.parent.observer.DidChangeAccount(a.parent, a)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return hasher.Receipt(0, len(hasher)-1), nil
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
