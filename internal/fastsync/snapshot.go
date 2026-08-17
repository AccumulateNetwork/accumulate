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
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Epoch identifies a fetched snapshot's position: the minor block whose
// state it holds, and — when the server runs DAG-BFT — the consensus round
// and committee epoch that committed that block, which a rejoining validator
// seeds its consensus state from. Round and CommitteeEpoch are reported by
// the server and are NOT covered by the state proof; a wrong value cannot
// corrupt verified state but can leave the node unable to rejoin until it
// resyncs from another peer.
type Epoch struct {
	Block          uint64
	Round          uint64
	CommitteeEpoch uint64

	// StateRoot is the state root the server computed at pin time. It is NOT
	// used for verification: the server's committed BPT lags the account
	// records by a block at the provable moment, so this is the previous
	// block's root and the directory never has it. The BVN sync proves the
	// root REBUILT from the restored accounts instead (RestoreSnapshot). Kept
	// for diagnostics only.
	StateRoot [32]byte
}

// FetchSnapshot pins a sync epoch on the server and streams the epoch's
// snapshot into w. The caller must verify the epoch block's anchor
// (Spine.AdvanceEpoch) before trusting the state, and RestoreSnapshot
// compares the rebuilt BPT root against the anchor's StateTreeAnchor.
func FetchSnapshot(ctx context.Context, svc private.SnapshotRanger, partition *url.URL, w io.Writer, opts ...private.SequenceOptions) (Epoch, error) {
	var so private.SequenceOptions
	if len(opts) > 0 {
		so = opts[0]
	}
	chunk, err := svc.SnapshotRange(ctx, partition, 0, 0, so)
	if err != nil {
		return Epoch{}, errors.UnknownError.WithFormat("pin snapshot: %w", err)
	}
	epoch := Epoch{Block: chunk.Block, Round: chunk.Round, CommitteeEpoch: chunk.Epoch, StateRoot: chunk.StateRoot}

	var offset uint64
	for {
		if chunk.Block != epoch.Block {
			return Epoch{}, errors.Conflict.WithFormat("the server re-pinned the epoch: %d became %d", epoch.Block, chunk.Block)
		}
		if chunk.Offset != offset {
			return Epoch{}, errors.Conflict.WithFormat("expected offset %d, got %d", offset, chunk.Offset)
		}
		_, err = w.Write(chunk.Data)
		if err != nil {
			return Epoch{}, errors.UnknownError.Wrap(err)
		}
		offset += uint64(len(chunk.Data))
		if offset >= chunk.Total {
			return epoch, nil
		}

		chunk, err = svc.SnapshotRange(ctx, partition, epoch.Block, offset, so)
		if err != nil {
			return Epoch{}, errors.UnknownError.WithFormat("fetch snapshot at %d: %w", offset, err)
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

// RestoreSnapshot restores a fetched snapshot into the database and returns the
// rebuilt state root: the BPT is rebuilt from the restored accounts' actual
// contents (restore re-hashes every account), so the returned root is the true
// state of the pinned block. It equals the StateTreeAnchor the partition's
// anchor carries and the directory records — which is why the caller proves the
// RETURNED root, not the server's reported one.
//
// The snapshot's own per-account hash check is skipped: the snapshot's BPT
// section reflects the server's committed BPT, which lags the account records
// by one block at the provable pin moment (the BPT for block N is only computed
// when block N+1 records the anchor). The rebuilt root recomputed here is block
// N's true state and subsumes the per-account check.
//
// If expectedRoot is non-zero the rebuilt root must equal it (the directory
// self-sync path, where the spine already carries the verified StateTreeAnchor).
// A zero expectedRoot skips the check — the BVN path proves the rebuilt root
// against the directory afterward instead. A non-zero expectedBlock binds the
// restored state to the claimed epoch block: the system ledger is in the BPT,
// so a server cannot pass off a different (even genuine) epoch as this one.
func RestoreSnapshot(db database.Beginner, file ioutil2.SectionReader, partition config.NetworkUrl, expectedRoot [32]byte, expectedBlock uint64) ([32]byte, error) {
	err := database.Restore(db, file, &database.RestoreOptions{SkipHashCheck: true})
	if err != nil {
		return [32]byte{}, errors.UnknownError.WithFormat("restore: %w", err)
	}

	batch := db.Begin(false)
	defer batch.Discard()
	root, err := batch.GetBptRootHash()
	if err != nil {
		return [32]byte{}, errors.UnknownError.WithFormat("compute state root: %w", err)
	}
	if expectedRoot != ([32]byte{}) && root != expectedRoot {
		return [32]byte{}, errors.Unauthenticated.WithFormat("restored state root %x does not match the verified state tree anchor %x", root, expectedRoot)
	}

	if expectedBlock != 0 {
		var ledger *protocol.SystemLedger
		err = batch.Account(partition.Ledger()).Main().GetAs(&ledger)
		if err != nil {
			return [32]byte{}, errors.UnknownError.WithFormat("load restored system ledger: %w", err)
		}
		if ledger.Index != expectedBlock {
			return [32]byte{}, errors.Unauthenticated.WithFormat("restored state is for block %d, not the pinned epoch block %d", ledger.Index, expectedBlock)
		}
	}
	return root, nil
}
