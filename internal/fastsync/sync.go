// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package fastsync

import (
	"context"
	"os"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
)

// spinePageSize is how many major headers are requested per page.
const spinePageSize = 256

// Client is the server surface Sync requires — the fast-sync extensions of
// the private sequencer service.
type Client interface {
	private.MajorHeaderRanger
	private.MinorRootRanger
	private.SnapshotRanger
}

// Options configures a fast sync.
type Options struct {
	// Client serves the fast-sync ranges (required).
	Client Client

	// Genesis is the trust-anchor state — the validator set and globals from
	// the pinned genesis snapshot (required).
	Genesis *network.GlobalValues

	// Partition is the partition to sync (required; currently must be the
	// directory).
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

	// Phase 1: walk the major-block spine from the trust anchor
	spine, err := NewSpine(opts.Genesis, 1)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	for {
		records, err := opts.Client.MajorHeaderRange(ctx, opts.Partition.URL, spine.NextMajor, spine.NextMajor+spinePageSize-1, private.SequenceOptions{})
		if err != nil {
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

	// Phase 2: bind the tail past the spine to the latest provable block
	for {
		r, err := opts.Client.MinorRootRange(ctx, opts.Partition.URL, spine.LastMinorBlock, 0, private.SequenceOptions{})
		switch {
		case err == nil:
			err = spine.AdvanceEpoch(r)
			if err != nil {
				return nil, errors.UnknownError.Wrap(err)
			}
			continue
		case retryable(err):
			// The tail is fully bound — nothing provable past this point yet
		default:
			return nil, errors.UnknownError.WithFormat("bind tail from %d: %w", spine.LastMinorBlock, err)
		}
		break
	}

	// Phase 3a: pin the sync epoch and fetch its snapshot. Pinning only
	// succeeds at a provable moment, so poll until the window opens.
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
		epoch, err = FetchSnapshot(ctx, opts.Client, opts.Partition.URL, file)
		_ = file.Close()
		if err == nil {
			break
		}
		if !retryable(err) {
			return nil, errors.UnknownError.WithFormat("fetch snapshot: %w", err)
		}
		err = poll(ctx)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	// Phase 3b: verify exactly up to the epoch block — its anchor is
	// recorded a block later and reaches quorum a few blocks after that
	for spine.LastMinorBlock < epoch.Block {
		r, err := opts.Client.MinorRootRange(ctx, opts.Partition.URL, spine.LastMinorBlock, epoch.Block, private.SequenceOptions{})
		switch {
		case err == nil:
			err = spine.AdvanceEpoch(r)
			if err != nil {
				return nil, errors.UnknownError.Wrap(err)
			}
		case retryable(err):
			err = poll(ctx)
			if err != nil {
				return nil, errors.UnknownError.Wrap(err)
			}
		default:
			return nil, errors.UnknownError.WithFormat("bind epoch block %d: %w", epoch.Block, err)
		}
	}
	if spine.LastMinorBlock != epoch.Block {
		return nil, errors.Conflict.WithFormat("the epoch block %d is not anchored exactly — verified position is %d", epoch.Block, spine.LastMinorBlock)
	}

	// Phase 3c: restore and prove the state
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	defer file.Close()
	err = RestoreSnapshot(opts.Database, file, opts.Partition, spine.StateTreeAnchor)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return &Result{Spine: spine, Epoch: epoch}, nil
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
