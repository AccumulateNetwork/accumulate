// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package fastsync

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/p2p"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// spinePageSize is how many major headers are requested per page.
const spinePageSize = 256

// Client is the server surface Sync requires — the fast-sync extensions of
// the private sequencer service.
type Client interface {
	private.MajorHeaderRanger
	private.MinorRootRanger
	private.SnapshotRanger
	private.PartitionRootRanger
}

// Options configures a fast sync.
type Options struct {
	// Client serves the fast-sync ranges (required).
	Client Client

	// Genesis is the trust-anchor state — the validator set and globals from
	// the pinned genesis snapshot (required).
	Genesis *network.GlobalValues

	// Partition is the partition to sync (required). Syncing a BVN walks the
	// DIRECTORY spine for trust and proves the BVN state root into it via
	// the directory's record of the BVN's anchors; Genesis must then hold
	// the directory trust anchor (every partition's genesis snapshot
	// carries the same network globals).
	Partition config.NetworkUrl

	// Database is the target to restore into (required).
	Database database.Beginner

	// SnapshotPath is where the fetched snapshot is staged. Defaults to a
	// temporary file.
	SnapshotPath string

	// Poll is called when the server reports a retryable condition (an
	// anchor not yet produced or without a quorum, or no provable pin
	// moment). It should wait for the network to advance. Defaults to a
	// one-second sleep.
	Poll func(ctx context.Context) error

	// NodeID, if set, directs every request to that specific peer instead of
	// routing by service discovery.
	NodeID p2p.PeerID
}

// Result reports what a fast sync verified and restored.
type Result struct {
	// Spine is the verified walk — its globals are the current validator set
	// and its roots are the verified position.
	Spine *Spine

	// Epoch is the restored position, including the consensus round and
	// committee epoch a rejoining validator seeds from (server-reported).
	Epoch Epoch
}

// Sync deploys a node's state from the network (#4058): it walks the
// major-block spine from the trust anchor, binds the tail to the present,
// fetches the epoch snapshot, restores it, and proves the restored state
// against the verified StateTreeAnchor. On success the database holds the
// complete, cryptographically verified state of the epoch block.
func Sync(ctx context.Context, opts Options) (*Result, error) {
	if opts.Client == nil || opts.Genesis == nil || opts.Database == nil || opts.Partition.URL == nil {
		return nil, errors.BadRequest.With("missing required sync options")
	}
	poll := opts.Poll
	if poll == nil {
		poll = func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
				return nil
			}
		}
	}

	// Phase 1: walk the DIRECTORY's major-block spine from the trust anchor.
	// The spine is the trust base for every partition — it is the only
	// self-anchoring walk, and it tracks the validator set by induction.
	dn := protocol.DnUrl()
	spine, err := NewSpine(opts.Genesis, 1)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	for {
		records, err := opts.Client.MajorHeaderRange(ctx, dn, spine.NextMajor, spine.NextMajor+spinePageSize-1, private.SequenceOptions{NodeID: opts.NodeID})
		if err != nil {
			if spine.NextMajor == 1 && errors.Code(err) == errors.NotFound {
				break // A young network — no major blocks yet; the epoch binding starts from genesis
			}
			return nil, errors.UnknownError.WithFormat("fetch major headers from %d: %w", spine.NextMajor, err)
		}
		for _, r := range records {
			err = spine.Advance(r)
			if err != nil {
				return nil, errors.UnknownError.Wrap(err)
			}
		}
		if len(records) < spinePageSize {
			break // Reached the end of the spine
		}
	}

	slog.Info("Fast sync: spine verified", "module", "fastsync", "lastMajor", spine.NextMajor-1, "block", spine.LastMinorBlock)

	// Phase 2: pin the sync epoch and fetch its snapshot. Pinning only
	// succeeds at a provable moment, so poll until the window opens. Binding
	// happens after — exactly to the pinned block — so the walk never chases
	// the moving tip.
	path := opts.SnapshotPath
	if path == "" {
		f, err := os.CreateTemp("", "fastsync-*.snapshot")
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		path = f.Name()
		_ = f.Close()
		defer func() { _ = os.Remove(path) }()
	}
	var epoch Epoch
	for {
		file, err := os.Create(path)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		epoch, err = FetchSnapshot(ctx, opts.Client, opts.Partition.URL, file, private.SequenceOptions{NodeID: opts.NodeID})
		_ = file.Close()
		if err == nil {
			break
		}
		if !retryable(err) {
			return nil, errors.UnknownError.WithFormat("fetch snapshot: %w", err)
		}
		slog.Info("Fast sync: waiting for a provable pin moment", "module", "fastsync")
		err = poll(ctx)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}
	slog.Info("Fast sync: snapshot fetched", "module", "fastsync", "block", epoch.Block, "round", epoch.Round)

	// Phase 3: verify the pinned state's root. For the directory the epoch
	// block is bound to the spine exactly and the anchor's StateTreeAnchor is
	// the verified root. For a BVN, the directory records every received
	// partition anchor's state root — one receipt proves the pinned root into
	// a directory block, and the spine binds to that block (#4058 phase 3b).
	var stateRoot [32]byte
	if dn.Equal(opts.Partition.URL) {
		err = bindDirectoryEpoch(ctx, opts, poll, spine, epoch.Block)
		if err != nil {
			return nil, err
		}
		stateRoot = spine.StateTreeAnchor
	} else {
		stateRoot, err = bindPartitionRoot(ctx, opts, poll, spine, &epoch)
		if err != nil {
			return nil, err
		}
	}

	slog.Info("Fast sync: restoring state", "module", "fastsync", "block", epoch.Block)

	// Phase 4: restore and prove the state
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	defer file.Close()
	err = RestoreSnapshot(opts.Database, file, opts.Partition, stateRoot, epoch.Block)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return &Result{Spine: spine, Epoch: epoch}, nil
}

// bindDirectoryEpoch advances the spine to exactly the epoch block — its
// anchor is recorded a block later and reaches quorum a few blocks after that.
func bindDirectoryEpoch(ctx context.Context, opts Options, poll func(context.Context) error, spine *Spine, target uint64) error {
	for spine.LastMinorBlock < target {
		r, err := opts.Client.MinorRootRange(ctx, protocol.DnUrl(), spine.LastMinorBlock, target, private.SequenceOptions{NodeID: opts.NodeID})
		switch {
		case err == nil:
			err = spine.AdvanceEpoch(r)
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
			slog.Info("Fast sync: bound", "module", "fastsync", "block", spine.LastMinorBlock, "target", target)
		case retryable(err):
			slog.Info("Fast sync: waiting for the epoch anchor", "module", "fastsync", "block", spine.LastMinorBlock, "target", target, "reason", err)
			err = poll(ctx)
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
		default:
			return errors.UnknownError.WithFormat("bind epoch block %d: %w", target, err)
		}
	}
	if spine.LastMinorBlock != target {
		return errors.Conflict.WithFormat("the epoch block %d is not anchored exactly — verified position is %d", target, spine.LastMinorBlock)
	}
	return nil
}

// bindPartitionRoot verifies a BVN's pinned state root against the directory
// (#4058 phase 3b): fetch the directory's receipt for the root, bind the
// spine to exactly the directory block the receipt ends at, and check the
// receipt chains the root to that block's verified root chain anchor. The
// server-reported root enters as a claim and leaves proven — the receipt only
// validates if the root is on the directory's record of the BVN's anchors,
// which only executing the BVN's quorum-signed anchors can append to.
func bindPartitionRoot(ctx context.Context, opts Options, poll func(context.Context) error, spine *Spine, epoch *Epoch) ([32]byte, error) {
	if epoch.StateRoot == ([32]byte{}) {
		return [32]byte{}, errors.NotAllowed.With("the server did not report the pinned block's state root — it may predate BVN fast sync")
	}

	// The anchor for the pinned block reaches the directory a few blocks
	// after the pin — poll until the receipt is available
	var record *private.PartitionRootRecord
	for {
		var err error
		record, err = opts.Client.PartitionRootRange(ctx, opts.Partition.URL, epoch.StateRoot, private.SequenceOptions{NodeID: opts.NodeID})
		if err == nil {
			break
		}
		if !retryable(err) {
			return [32]byte{}, errors.UnknownError.WithFormat("fetch partition root receipt: %w", err)
		}
		slog.Info("Fast sync: waiting for the pinned anchor to reach the directory", "module", "fastsync", "block", epoch.Block, "reason", err)
		err = poll(ctx)
		if err != nil {
			return [32]byte{}, errors.UnknownError.Wrap(err)
		}
	}
	if record.Receipt == nil {
		return [32]byte{}, errors.BadRequest.With("incomplete partition root record")
	}

	// Bind the spine to exactly the directory block the receipt ends at
	err := bindDirectoryEpoch(ctx, opts, poll, spine, record.DirectoryBlock)
	if err != nil {
		return [32]byte{}, err
	}

	// The receipt must chain the pinned state root to the verified root
	if !bytes.Equal(record.Receipt.Start, epoch.StateRoot[:]) {
		return [32]byte{}, errors.Unauthenticated.With("the receipt does not start at the pinned state root")
	}
	if !bytes.Equal(record.Receipt.Anchor, spine.RootChainAnchor[:]) {
		return [32]byte{}, errors.Unauthenticated.With("the receipt does not end at the verified directory root")
	}
	if !record.Receipt.Validate(nil) {
		return [32]byte{}, errors.Unauthenticated.With("invalid partition root receipt")
	}

	slog.Info("Fast sync: partition root proven into the directory", "module", "fastsync",
		"partition", opts.Partition.URL, "block", epoch.Block, "directoryBlock", record.DirectoryBlock)
	return epoch.StateRoot, nil
}

// retryable reports whether the error means "the network has not advanced
// far enough yet" — the anchor is not produced, lacks a quorum, or there is
// no provable pin moment.
func retryable(err error) bool {
	switch errors.Code(err) {
	case errors.NotReady, errors.NotFound:
		return true
	}
	return false
}
