// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package fastsync

import (
	"context"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// FetchSnapshot pins a sync epoch on the server and streams the epoch's
// snapshot into w. It returns the epoch's block — the caller must verify
// that block's anchor (Spine.AdvanceEpoch) before trusting the state, and
// RestoreSnapshot compares the rebuilt BPT root against the anchor's
// StateTreeAnchor.
func FetchSnapshot(ctx context.Context, svc private.SnapshotRanger, partition *url.URL, w io.Writer) (uint64, error) {
	chunk, err := svc.SnapshotRange(ctx, partition, 0, 0, private.SequenceOptions{})
	if err != nil {
		return 0, errors.UnknownError.WithFormat("pin snapshot: %w", err)
	}
	epoch := chunk.Block

	var offset uint64
	for {
		if chunk.Block != epoch {
			return 0, errors.Conflict.WithFormat("the server re-pinned the epoch: %d became %d", epoch, chunk.Block)
		}
		if chunk.Offset != offset {
			return 0, errors.Conflict.WithFormat("expected offset %d, got %d", offset, chunk.Offset)
		}
		_, err = w.Write(chunk.Data)
		if err != nil {
			return 0, errors.UnknownError.Wrap(err)
		}
		offset += uint64(len(chunk.Data))
		if offset >= chunk.Total {
			return epoch, nil
		}

		chunk, err = svc.SnapshotRange(ctx, partition, epoch, offset, private.SequenceOptions{})
		if err != nil {
			return 0, errors.UnknownError.WithFormat("fetch snapshot at %d: %w", offset, err)
		}
	}
}

// LoadGenesisGlobals extracts the trust-anchor state — the validator set and
// network globals — from a genesis snapshot, the walk's only out-of-band
// trust input.
func LoadGenesisGlobals(file ioutil2.SectionReader, partition config.NetworkUrl) (*network.GlobalValues, error) {
	db := database.OpenInMemory(nil)
	err := snapshot.FullRestore(db, file, nil, partition)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("restore genesis snapshot: %w", err)
	}
	g := new(network.GlobalValues)
	err = db.View(func(batch *database.Batch) error {
		return g.Load(partition.URL, func(account *url.URL, target interface{}) error {
			return batch.Account(account).Main().GetAs(target)
		})
	})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load genesis globals: %w", err)
	}
	return g, nil
}

// RestoreSnapshot restores a fetched snapshot into the database and verifies
// it: the BPT is rebuilt from the restored accounts' actual contents (restore
// re-hashes every account), so if its root equals the quorum-verified
// StateTreeAnchor of the epoch's anchor, every account state is proven into
// the verified spine.
func RestoreSnapshot(db database.Beginner, file ioutil2.SectionReader, partition config.NetworkUrl, expectedRoot [32]byte) error {
	err := snapshot.FullRestore(db, file, nil, partition)
	if err != nil {
		return errors.UnknownError.WithFormat("restore: %w", err)
	}

	batch := db.Begin(false)
	defer batch.Discard()
	root, err := batch.GetBptRootHash()
	if err != nil {
		return errors.UnknownError.WithFormat("compute state root: %w", err)
	}
	if root != expectedRoot {
		return errors.Unauthenticated.WithFormat("restored state root %x does not match the verified state tree anchor %x", root, expectedRoot)
	}
	return nil
}
