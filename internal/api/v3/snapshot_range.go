// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"io"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var _ private.SnapshotRanger = (*Sequencer)(nil)

// SnapshotChunkSize is the size of one snapshot stream chunk.
const SnapshotChunkSize = 256 << 10

// pinnedSnapshot is a state snapshot collected at a sync epoch (#4058). The
// consistency pin only needs to last for the collection — the file is
// immutable afterward, so serving needs no live database view.
type pinnedSnapshot struct {
	block uint64
	file  *os.File
	size  uint64
}

// SnapshotRange implements [private.SnapshotRanger.SnapshotRange]. Epoch
// zero pins the sync epoch — the state is collected into a temporary file
// from a view captured at a provable moment, so the snapshot is consistent
// as of one anchored block. Any other epoch must match the pinned one; a
// mismatch means the pin was replaced and the client must start over.
func (s *Sequencer) SnapshotRange(ctx context.Context, partition *url.URL, epoch, offset uint64, _ private.SequenceOptions) (*private.SnapshotChunk, error) {
	if partition == nil {
		return nil, errors.BadRequest.With("missing partition")
	}
	if !s.partition.URL.Equal(partition) {
		return nil, errors.BadRequest.WithFormat("requested partition is %s but this partition is %s", partition, s.partitionID)
	}

	s.snapMu.Lock()
	defer s.snapMu.Unlock()

	if epoch == 0 {
		err := s.pinSnapshot()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("pin snapshot: %w", err)
		}
	} else if s.snap == nil || s.snap.block != epoch {
		return nil, errors.NotFound.WithFormat("epoch %d is not pinned — restart from epoch zero", epoch)
	}

	if offset > s.snap.size {
		return nil, errors.BadRequest.WithFormat("offset %d exceeds snapshot size %d", offset, s.snap.size)
	}
	n := s.snap.size - offset
	if n > SnapshotChunkSize {
		n = SnapshotChunkSize
	}
	data := make([]byte, n)
	_, err := s.snap.file.ReadAt(data, int64(offset))
	if err != nil && err != io.EOF {
		return nil, errors.UnknownError.WithFormat("read snapshot: %w", err)
	}

	chunk := &private.SnapshotChunk{
		Block:  s.snap.block,
		Total:  s.snap.size,
		Offset: offset,
		Data:   data,
	}
	if c, ok := s.commitRoundFor(s.snap.block); ok {
		chunk.Round, chunk.Epoch = c.round, c.epoch
	}
	return chunk, nil
}

// captureProvableView is called synchronously from the DidCommitBlock event.
// The snapshot is only provable if some anchor will attest its exact state:
// anchor roots are populated when the anchor is RECORDED — at the start of
// the block after it was prepared — from precisely the state as of its own
// block. So the provable moment is when the system ledger holds a freshly
// prepared anchor for the block that just committed. At block rates the
// window is one block wide, far too narrow to catch by polling, so the
// commit event — which fires synchronously, before the next block can begin
// — captures a read view of it. The view is a point-in-time database
// transaction, replaced at every provable commit and only consumed by
// pinSnapshot.
func (s *Sequencer) captureProvableView(index uint64) {
	beginner, ok := s.db.(database.Beginner)
	if !ok {
		return
	}

	batch := beginner.Begin(false)
	var ledger *protocol.SystemLedger
	err := batch.Account(s.partition.Ledger()).Main().GetAs(&ledger)
	if err != nil || ledger.Anchor == nil || ledger.Index != index {
		batch.Discard()
		return
	}
	anchor, ok := ledger.Anchor.(protocol.AnchorBody)
	if !ok || anchor.GetPartitionAnchor().MinorBlockIndex != ledger.Index {
		batch.Discard()
		return
	}

	s.viewMu.Lock()
	if s.provable != nil {
		s.provable.Discard()
	}
	s.provable = batch
	s.provableBlock = ledger.Index
	s.viewMu.Unlock()
}

func (s *Sequencer) pinSnapshot() error {
	// Take ownership of the captured provable view. When none was captured —
	// no commit event carried a round (CometBFT) or no recent block prepared
	// an anchor — fall back to checking whether the current committed state
	// happens to be provable right now.
	s.viewMu.Lock()
	batch, block := s.provable, s.provableBlock
	s.provable = nil
	s.viewMu.Unlock()

	if batch == nil {
		beginner, ok := s.db.(database.Beginner)
		if !ok {
			return errors.NotReady.With("no provable state captured yet — retry after the next block that prepares an anchor")
		}
		batch = beginner.Begin(false)
		var ledger *protocol.SystemLedger
		err := batch.Account(s.partition.Ledger()).Main().GetAs(&ledger)
		switch {
		case err != nil:
			batch.Discard()
			return errors.UnknownError.WithFormat("load system ledger: %w", err)
		case ledger.Anchor == nil:
			batch.Discard()
			return errors.NotReady.With("no anchor is pending — retry after the next block that prepares one")
		}
		anchor, ok := ledger.Anchor.(protocol.AnchorBody)
		if !ok || anchor.GetPartitionAnchor().MinorBlockIndex != ledger.Index {
			batch.Discard()
			return errors.NotReady.With("the pending anchor is stale — retry after the next block that prepares one")
		}
		block = ledger.Index
	}
	defer batch.Discard()

	file, err := os.CreateTemp("", "fastsync-snapshot-*")
	if err != nil {
		return errors.UnknownError.WithFormat("create temporary file: %w", err)
	}

	_, err = batch.Collect(file, s.partition.URL, &database.CollectOptions{})
	if err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return err
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return errors.UnknownError.WithFormat("stat snapshot: %w", err)
	}

	if s.snap != nil {
		_ = s.snap.file.Close()
		_ = os.Remove(s.snap.file.Name())
	}
	s.snap = &pinnedSnapshot{block: block, file: file, size: uint64(info.Size())}
	return nil
}
