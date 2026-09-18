// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package private

import (
	"context"
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// MaxSnapshotPages bounds how many pages one fetch will take. A page covers
// at most a few hundred sequence numbers, so an honest peer finishes a stage
// of a million entries well inside this; a peer whose cursor crawls forward
// one number at a time does not, and without a bound the reader would follow
// it forever (#4291 review).
const MaxSnapshotPages = 4096

// MaxSnapshotRestarts bounds how many times one fetch will start over
// because the peer committed a block while it was reading. The peer commits
// a block every second or so and does not pin a version of its stage for a
// reader, so a stage that takes longer than a block interval to send can
// restart — but not without end. When the restarts run out the peer is too
// busy to be read from, and the join asks another validator or tries again
// (#4294).
const MaxSnapshotRestarts = 3

// snapshotRetryDelay is how long the reader waits before starting over, and
// it doubles each time: starting over at once against a peer that is
// committing blocks faster than it can answer only makes both busier.
const snapshotRetryDelay = 50 * time.Millisecond

// FetchStagingSnapshot assembles a peer's whole staging from the pages it
// serves. Every page must be as of the same block: the peer's staging moves
// on between calls, and pages from two blocks are not a state any node ever
// held, so a reader that assembled them would execute a block no one else
// executed (executor.md, "Sync" step 2). A page as of a different block
// makes the reader discard what it has and start over, up to
// MaxSnapshotRestarts times.
//
// The result is one snapshot, its streams in the order the pages carried
// them — a stream split across pages appears once per page, which is how
// staging takes it in.
//
// It returns errors.NotReady, naming the peer and the block, when the fetch
// cannot be completed: too many pages, or too many restarts. That is a
// statement about this peer at this moment and not about the snapshot, so
// the caller asks another validator or tries again (#4294).
func FetchStagingSnapshot(ctx context.Context, from StagingSnapshotter, partition string) (*StagingSnapshot, error) {
	var block uint64
	for restart := 0; ; restart++ {
		if restart > 0 {
			if restart > MaxSnapshotRestarts {
				return nil, errors.NotReady.WithFormat("%s staging from %s: the block moved %d times while it was being read (last %d)", partition, peerName(from), restart-1, block)
			}
			select {
			case <-ctx.Done():
				return nil, errors.UnknownError.Wrap(ctx.Err())
			case <-time.After(snapshotRetryDelay << (restart - 1)):
			}
		}

		whole, moved, err := fetchStagingSnapshot(ctx, from, partition)
		switch {
		case err != nil:
			return nil, errors.UnknownError.Wrap(err)
		case moved == 0:
			return whole, nil
		}
		block = moved
	}
}

// fetchStagingSnapshot reads one whole snapshot. When the peer commits a
// block part way through it returns the block it moved to and no snapshot,
// which is the caller's cue to start over.
func fetchStagingSnapshot(ctx context.Context, from StagingSnapshotter, partition string) (*StagingSnapshot, uint64, error) {
	req := &StagingSnapshotRequest{Partition: partition}
	whole := new(StagingSnapshot)
	for i := 0; ; i++ {
		if i >= MaxSnapshotPages {
			return nil, 0, errors.NotReady.WithFormat("%s staging from %s at block %d: more than %d pages", partition, peerName(from), whole.Block, MaxSnapshotPages)
		}

		page, err := from.StagingSnapshot(ctx, req)
		if err != nil {
			return nil, 0, errors.UnknownError.Wrap(err)
		}
		if i == 0 {
			whole.Block = page.Block
		} else if page.Block != whole.Block {
			return nil, page.Block, nil
		}
		whole.Streams = append(whole.Streams, page.Streams...)
		if !page.More {
			return whole, 0, nil
		}

		// A cursor is a position, and every position has a source: a page
		// that says there is more but does not say where is a peer this
		// reader cannot follow.
		if page.NextSource == nil {
			return nil, 0, errors.PeerMisbehaved.WithFormat("%s staging from %s: the next page's cursor names no source", partition, peerName(from))
		}

		// A cursor that does not move is a server that will never finish.
		if sameStagingCursor(req, page) {
			return nil, 0, errors.PeerMisbehaved.WithFormat("%s staging from %s: paging did not advance past %v %d", partition, peerName(from), page.NextLedger, page.NextNumber)
		}
		req.Ledger, req.Source = page.NextLedger, page.NextSource
		req.Number, req.ProofOffset = page.NextNumber, page.NextProofOffset
	}
}

// sameStagingCursor reports whether the page sends the reader back to where
// it already was. Ledger is compared nil-tolerantly: a source that holds
// proofs and no stream is a cursor with no ledger.
func sameStagingCursor(req *StagingSnapshotRequest, page *StagingSnapshot) bool {
	if req.Source == nil || !req.Source.Equal(page.NextSource) {
		return false
	}
	if (req.Ledger == nil) != (page.NextLedger == nil) {
		return false
	}
	if req.Ledger != nil && !req.Ledger.Equal(page.NextLedger) {
		return false
	}
	return req.Number == page.NextNumber && req.ProofOffset == page.NextProofOffset
}

// peerName is what an error calls the peer it is about: what the peer calls
// itself when it can say, and its type when it cannot.
func peerName(from StagingSnapshotter) string {
	if s, ok := from.(fmt.Stringer); ok {
		return s.String()
	}
	return fmt.Sprintf("%T", from)
}
