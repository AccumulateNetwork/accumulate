// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"bytes"
	"context"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ErrNotAnchored says the Directory has not anchored the block a peer served
// an account at. It is a wait, not a failure: the anchor for a block arrives a
// few blocks later, and the caller asks again.
var ErrNotAnchored = errors.NotReady.With("the directory has not anchored that block yet")

// Verifier says what BPT root the Directory anchored for a partition's block.
// It is the only root a pulled account is verified against: a node that is
// still pulling has no root of its own to trust, and a peer's word for its own
// root is worth nothing (executor.md, "Sync").
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

// DirectoryAnchors reads the roots the Directory anchored out of
// acc://dn.acme/anchors, through the API. Every anchor executed at the
// Directory — a BVN's BlockValidatorAnchor and the Directory's own
// DirectoryAnchor — carries the source partition's StateTreeAnchor for one of
// its blocks, which is the root that block's state hashes to.
//
// It reads forward and remembers what it has read, so a pull that walks the
// network block by block reads each anchor once.
type DirectoryAnchors struct {
	// Query reaches the Directory.
	Query api.Querier

	// PageSize is how many anchor-chain entries are read per call. Default 64.
	PageSize uint64

	// OnAnchor, if set, is called for every anchor read, in chain order. The
	// tracker's Observe is the intended consumer.
	OnAnchor func(partition *url.URL, block uint64, root [32]byte)

	mu    sync.Mutex
	roots map[anchorKey][32]byte
	next  uint64 // the next main-chain entry to read
}

type anchorKey struct {
	partition string
	block     uint64
}

// AnchoredRoot returns the BPT root the Directory anchored for the partition's
// block, reading forward from where it last stopped. It returns ErrNotAnchored
// when the Directory has not executed that anchor yet.
func (d *DirectoryAnchors) AnchoredRoot(ctx context.Context, partition *url.URL, block uint64) ([32]byte, error) {
	key := anchorKey{partition.String(), block}

	d.mu.Lock()
	defer d.mu.Unlock()

	if root, ok := d.roots[key]; ok {
		return root, nil
	}
	if err := d.readLocked(ctx); err != nil {
		return [32]byte{}, errors.UnknownError.Wrap(err)
	}
	if root, ok := d.roots[key]; ok {
		return root, nil
	}
	return [32]byte{}, errors.NotReady.WithFormat("%v block %d: %w", partition, block, ErrNotAnchored)
}

// Read reads whatever anchors the Directory has executed since the last call,
// calling OnAnchor for each. A caller that is only feeding a tracker uses this
// instead of asking for a specific block.
func (d *DirectoryAnchors) Read(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.readLocked(ctx)
}

func (d *DirectoryAnchors) readLocked(ctx context.Context) error {
	if d.roots == nil {
		d.roots = map[anchorKey][32]byte{}
	}
	pageSize := d.PageSize
	if pageSize == 0 {
		pageSize = 64
	}

	q := api.Querier2{Querier: d.Query}
	pool := protocol.DnUrl().JoinPath(protocol.AnchorPool)
	for {
		count, expand := pageSize, true
		page, err := q.QueryMainChainEntries(ctx, pool, &api.ChainQuery{
			Name:  "main",
			Range: &api.RangeOptions{Start: d.next, Count: &count, Expand: &expand},
		})
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, errors.NotFound):
			return nil // Nothing new
		default:
			return errors.UnknownError.WithFormat("read the directory's anchors: %w", err)
		}
		if page == nil || len(page.Records) == 0 {
			return nil
		}

		for _, rec := range page.Records {
			d.next = rec.Index + 1
			if rec.Value == nil || rec.Value.Message == nil || rec.Value.Message.Transaction == nil {
				continue
			}
			body, ok := rec.Value.Message.Transaction.Body.(protocol.AnchorBody)
			if !ok {
				continue // The anchor pool holds other transactions too
			}
			a := body.GetPartitionAnchor()
			if a == nil || a.Source == nil {
				continue
			}
			d.roots[anchorKey{a.Source.String(), a.MinorBlockIndex}] = a.StateTreeAnchor
			if d.OnAnchor != nil {
				d.OnAnchor(a.Source, a.MinorBlockIndex, a.StateTreeAnchor)
			}
		}

		if uint64(len(page.Records)) < count {
			return nil
		}
	}
}
