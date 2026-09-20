// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"bytes"
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// ErrNotAnchored says the Directory has not anchored the block a peer served
// an account at. It is a wait, not a failure: the anchor for a block arrives a
// few blocks later, and the caller asks again.
var ErrNotAnchored = errors.NotReady.With("the directory has not anchored that block yet")

// Verifier says what BPT root was anchored for a partition's block, and
// vouches that a quorum of that partition's validators signed it. It is the
// only root a pulled account is verified against: a node that is still
// pulling has no root of its own to trust, and a peer's word for its own root
// is worth nothing (executor.md, "Sync").
//
// The implementation is internal/core/bootstrap/anchorsrc. This package
// states the interface and does not reach for the anchors itself: the reader
// that lived here recorded every StateTreeAnchor it could decode, with no
// signature checked, so the root a pulled account was verified against was a
// number a peer sent (#4301).
type Verifier interface {
	AnchoredRoot(ctx context.Context, partition *url.URL, block uint64) ([32]byte, error)
}

// Verify reports whether the account state in batch is the state that hashes
// into anchoredRoot, given the receipt the peer served with it.
//
// The peer's receipt runs from the account's state hash to the peer's BPT
// root. Three things must hold: the receipt must be internally valid, it must
// end at the root the Directory anchored, and it must pass through the leaf
// the pulled state hashes to locally. The third is what makes the pull safe —
// without it a peer could serve a true receipt for an account and a false body
// for it.
func Verify(batch *database.Batch, u *url.URL, receipt *api.Receipt, anchoredRoot [32]byte) error {
	if receipt == nil {
		return errors.BadRequest.WithFormat("%v: the peer served no receipt", u)
	}

	if !receipt.Receipt.Validate(nil) {
		return errors.BadRequest.WithFormat("%v: the peer's receipt does not validate", u)
	}
	if !bytes.Equal(receipt.Receipt.Anchor, anchoredRoot[:]) {
		return errors.Conflict.WithFormat(
			"%v: the peer's receipt ends at %x, and the directory anchored %x",
			u, receipt.Receipt.Anchor, anchoredRoot[:])
	}

	// A HISTORICAL receipt starts at a plain hash of the main state, and a
	// CURRENT one starts at the account's whole BPT entry. The check has to
	// match, and using the wrong one refuses an honest answer.
	//
	// The account hasher covers the main state, the directory list, the
	// chains and the pending list. A peer answering as of a past block serves
	// the body for that block and clears the rest, because it does not retain
	// them (internal/api/v3/querier.go, historicalStateReceipt) -- so the
	// puller holds that block's body beside ITS OWN directory, chains and
	// pending list, which are still the ones it stopped with. Recomputing the
	// whole entry from that mixture hashes something no node ever held, and
	// every account whose chains moved -- the ledger, the anchors, the
	// synthetic ledger, which is exactly what a restart needs -- is refused
	// (#4362). So on a historical answer the state is checked where the proof
	// actually starts.
	if receipt.StartsAtMainState {
		state, err := batch.Account(u).Main().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("%v: load the pulled state: %w", u, err)
		}
		want, err := database.MainStateHash(state)
		if err != nil {
			return errors.UnknownError.WithFormat("%v: hash the pulled state: %w", u, err)
		}
		if !bytes.Equal(receipt.Receipt.Start, want[:]) {
			return errors.Conflict.WithFormat(
				"%v: the body served hashes to %x and its receipt starts at %x",
				u, want[:], receipt.Receipt.Start)
		}
		return nil
	}

	local, err := batch.Account(u).StateTreeReceipt()
	if err != nil {
		return errors.UnknownError.WithFormat("%v: hash the pulled state: %w", u, err)
	}
	if !receipt.Receipt.Contains(local) {
		return errors.Conflict.WithFormat(
			"%v: the state served does not hash into the anchored root", u)
	}
	return nil
}
