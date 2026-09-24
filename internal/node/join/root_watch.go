// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/tracker"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// HandedOff records the block the node handed off at. The blocks after it are
// the ones this node executes, and the only ones Diverged checks: below it the
// local bpt chain holds what the pull left there, not what this node computed,
// and comparing an anchor from before the handoff against the first root
// recorded after it would report a divergence that is not there.
//
// The state handed off at may be unproven: once the walk is done the node
// executes and compares at every block that anchors (executor spec, "Sync",
// "Execute, and repair on a mismatch"). From here executed is the last block
// compared, or the block handed off at.
func (s *PulledState) HandedOff(q uint64) {
	s.executed = q
	s.sync = nil
	s.repairFrom = 0
}

// Diverged compares the root this node computed after each block it executed
// since the handoff with the partition's signed anchor for that block, at
// every block that sent one (executor spec, "Sync", "Execute, and repair on a
// mismatch"). A match is the proof, step 3: a node that handed off from an
// unproven state is promoted there. A mismatch is reported, and the next pull
// is a repair from the block ledger, from the last block that matched -- or
// from the start of the pull, if none has.
//
// The local root after block N is not read from the BPT, which moves with
// every commit and would be compared against an anchor of another block. It is
// read from the bpt chain: every non-empty block M records the root after
// M - 1 there, and indexes it at M. The first index entry after N is that of
// the next non-empty block, and every block between the two is empty and
// changed nothing, so its entry is the root after N. A block with no entry
// after it has not been followed by one yet, and is checked on a later call.
func (s *PulledState) Diverged(ctx context.Context) (uint64, bool, error) {
	err := s.readAnchors(ctx)
	if err != nil {
		return 0, false, errors.UnknownError.WithFormat("read this partition's anchors: %w", err)
	}

	obs := s.tracker.Snapshot()
	sort.Slice(obs, func(i, j int) bool { return obs[i].Block < obs[j].Block })

	batch := s.db.Begin(false)
	defer batch.Discard()
	bpt := batch.Account(s.partition.JoinPath(protocol.Ledger)).BptChain()

	for _, o := range obs {
		if o.Block <= s.executed {
			continue
		}
		local, ok, err := rootAfter(bpt, o.Block)
		if err != nil {
			return 0, false, errors.UnknownError.WithFormat("read this node's root after block %d: %w", o.Block, err)
		}
		if !ok {
			// Not followed by an executed block yet, and neither is any
			// block after it.
			break
		}
		if local != o.Anchor {
			s.log.Warn("An executed block's root is not its anchored root; repairing from the block ledger",
				"partition", s.partition, "block", o.Block, "compared", s.executed, "matched", s.provenAt)
			s.sync = nil
			s.repairFrom = s.provenAt
			if s.repairFrom == 0 {
				s.repairFrom = s.pullFrom
			}
			return o.Block, true, nil
		}
		s.executed = o.Block
		s.provenAt = o.Block
		if s.machine.State() != nodestate.StateActive {
			s.matched = tracker.Match{Block: o.Block, Anchor: o.Anchor}
			s.Promote(o.Block)
		}
	}
	return 0, false, nil
}

// rootAfter is the root this node's state had after block n, from the bpt
// chain, and whether a block after n has recorded it yet.
func rootAfter(bpt *database.Chain2, n uint64) ([32]byte, bool, error) {
	index := bpt.Index()
	head, err := index.Head().Get()
	if err != nil {
		return [32]byte{}, false, errors.UnknownError.Wrap(err)
	}
	if head.Count == 0 {
		return [32]byte{}, false, nil
	}
	_, entry, err := indexing.SearchIndexChain2(index, 0, indexing.MatchAfter, indexing.SearchIndexChainByBlock(n+1))
	switch {
	case errors.Is(err, indexing.ErrReachedChainEnd):
		return [32]byte{}, false, nil
	case err != nil:
		return [32]byte{}, false, errors.UnknownError.Wrap(err)
	}
	hash, err := bpt.Inner().Entry(int64(entry.Source))
	if err != nil {
		return [32]byte{}, false, errors.UnknownError.WithFormat("bpt chain entry %d: %w", entry.Source, err)
	}
	if len(hash) != 32 {
		return [32]byte{}, false, errors.InternalError.WithFormat("bpt chain entry %d is %d bytes", entry.Source, len(hash))
	}
	return [32]byte(hash), true, nil
}
