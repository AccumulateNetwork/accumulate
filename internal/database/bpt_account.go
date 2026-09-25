// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"bytes"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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
	// The state tree holds no leaf for an account that holds nothing
	// (executor spec, invariant 13; #4437). A write to a missing account's
	// bookkeeping alone — a transaction's votes or payments, a vote recorded
	// against a principal that does not exist — marks it dirty, and without
	// this it got a leaf hashing to nothing, identical for every such
	// account. Only an account that holds NOTHING is skipped. One with no main
	// state that holds something else — a chain, a pending transaction, a
	// directory entry — keeps its leaf, so no change is hidden from the tree,
	// and is counted and logged: it is a write that should not have happened
	// (RecordHistory no longer makes one).
	_, err := a.Main().Get()
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		blank, err := a.holdsNothing()
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
		if blank {
			return nil
		}
		mStatelessAccountLeaves.Inc()
		a.parent.logger.Error("An account with no main state holds state; its leaf is kept", "account", a.Url())
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
//
// It returns nil when there is no such path: the debug observer collapses the
// components into one hash, and a one-element "receipt" starts at that hash -
// the whole leaf - not at a hash of the main state, so it would claim a
// main-state start it is not.
func (a *Account) StateTreeReceipt() (*merkle.Receipt, error) {
	hasher, err := a.parent.observer.DidChangeAccount(a.parent, a)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if len(hasher) < 2 {
		return nil, nil
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

// holdsNothing is whether every part of the account its leaf hashes is empty
// apart from the main state: no chain, no pending transaction, no directory
// entry.
func (a *Account) holdsNothing() (bool, error) {
	chains, err := a.Chains().Get()
	if err != nil {
		return false, errors.UnknownError.WithFormat("load chains: %w", err)
	}
	if len(chains) > 0 {
		return false, nil
	}
	pending, err := a.Pending().Get()
	if err != nil {
		return false, errors.UnknownError.WithFormat("load pending: %w", err)
	}
	if len(pending) > 0 {
		return false, nil
	}
	dir, err := a.Directory().Get()
	if err != nil {
		return false, errors.UnknownError.WithFormat("load directory: %w", err)
	}
	return len(dir) == 0, nil
}

// mStatelessAccountLeaves counts leaves written for an account that has no
// main state and holds something else. Any non-zero count is a defect: some
// write gave a missing account state (#4437).
var mStatelessAccountLeaves = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: "accumulate",
	Subsystem: "database",
	Name:      "stateless_account_leaves_total",
	Help:      "State-tree leaves written for an account with no main state that holds a chain, a pending transaction or a directory entry; any non-zero count is a defect",
})
