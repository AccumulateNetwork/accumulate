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

	local, err := batch.Account(u).StateTreeReceipt()
	if err != nil {
		return errors.UnknownError.WithFormat("%v: hash the pulled state: %w", u, err)
	}

	if !receipt.Receipt.Validate(nil) {
		return errors.BadRequest.WithFormat("%v: the peer's receipt does not validate", u)
	}
	if !bytes.Equal(receipt.Receipt.Anchor, anchoredRoot[:]) {
		return errors.Conflict.WithFormat(
			"%v: the peer's receipt ends at %x, and the directory anchored %x",
			u, receipt.Receipt.Anchor, anchoredRoot[:])
	}
	if !receipt.Receipt.Contains(local) {
		return errors.Conflict.WithFormat(
			"%v: the state served does not hash into the anchored root", u)
	}
	return nil
}
