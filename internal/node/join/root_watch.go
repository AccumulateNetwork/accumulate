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
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/tracker"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
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
// "Two mismatches"). From here executed is the last block
// compared, or the block handed off at.
func (s *PulledState) HandedOff(q uint64) {
	s.executed = q
	if s.sync != nil {
		s.served = s.sync.servedBy
	}
	s.sync = nil
	s.repairFrom = 0
}

// Diverged compares the root this node computed after each block it executed
// since the handoff with the partition's signed anchor for that block, at
// every block that sent one (executor spec, "Sync", "Two
// mismatches"). A match is the proof, step 3: a node that handed off from an
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
	proven := false
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
			// Since the last comparison: the last block whose root was
			// compared and matched, or the block the node handed off at
			// (executor spec, "Sync", "Two mismatches").
			// A repair that does not bring a match is followed by one that
			// walks the tree again (startRepair).
			s.RepairFrom(s.executed)
			return o.Block, true, nil
		}
		s.executed = o.Block
		s.provenAt = o.Block
		s.repairs = 0
		s.served = nil // what it took is proven
		proven = true
		if s.machine.State() != nodestate.StateActive {
			s.matched = tracker.Match{Block: o.Block, Anchor: o.Anchor}
			s.Promote(o.Block)
		}
	}
	batch.Discard()

	// The state is proven at the last block that matched, and with it the
	// network definition it holds: the trusted sets move, and a restart
	// trusts them (#4438, review in passing). Without this a node proven
	// here, and not by Matched, would verify with the sets it started with
	// for as long as it runs, and a change to them would blind the watch.
	if proven {
		s.refreshAuthority()
	}

	// Past the match, what the join took by its chain heads alone is
	// brought in whole, a few accounts a check, while the node executes.
	if s.machine.State() == nodestate.StateActive {
		s.backfill(ctx)
	}
	return 0, false, nil
}

// RepairFrom makes the next pull a repair from the block ledger, from block
// on (executor spec, "Sync", "Two mismatches"): every account the
// partition's records name after block, and every account this node's own
// records name through the block it last executed, is pulled again whole --
// main state, every chain with its entries and the messages behind them,
// pending and directory -- a chain the node grew wrongly is replaced by the
// peers', and an account no peer holds is deleted. The node calls it when an
// executed block's root differs from the partition's signed anchor; block is
// the last block whose root matched, or where the pull began. It must not be
// executing while the repair pulls: the caller collects first (join.Run).
func (s *PulledState) RepairFrom(block uint64) {
	// The pull handed off from did not bring the match: the peers whose
	// answers it took go to the back of the order.
	if s.demoted == nil {
		s.demoted = map[string]int{}
	}
	for peer := range s.served {
		s.demoted[peer]++
	}
	s.served = nil

	// Zero is nothing to repair from: the next pull starts afresh.
	s.sync = nil
	s.repairFrom = block
}

// backfillPerCheck is how many head-only accounts one root check backfills.
const backfillPerCheck = 64

// backfill brings in the entries below the open mark set of accounts the join
// took by their chain heads alone (pull.Backfill). It writes only below each
// chain's head, so it runs while the node executes. An account that cannot be
// backfilled now is tried again on a later check.
func (s *PulledState) backfill(ctx context.Context) {
	n := 0
	for k, u := range s.headOnly {
		if n >= backfillPerCheck || ctx.Err() != nil {
			return
		}
		n++
		if s.backfillOne(ctx, u) {
			s.unmarkHeadOnly(k)
		}
	}
}

// markHeadOnly records that u was taken by its chain heads alone: in memory,
// and under SystemData, where no pull reaches, so that a restart before the
// backfill ends still backfills it. The next pull would not name it again:
// its leaf is the peers'.
func (s *PulledState) markHeadOnly(u *url.URL) {
	k := accountKey(u)
	if _, ok := s.headOnly[k]; ok {
		return
	}
	if s.headOnly == nil {
		s.headOnly = map[[32]byte]*url.URL{}
	}
	s.headOnly[k] = u
	s.writeHeadOnly(u, func(set values.Set[*url.URL]) error { return set.Add(u) })
}

// unmarkHeadOnly records that the account keyed k is held whole.
func (s *PulledState) unmarkHeadOnly(k [32]byte) {
	u, ok := s.headOnly[k]
	if !ok {
		return
	}
	delete(s.headOnly, k)
	s.writeHeadOnly(u, func(set values.Set[*url.URL]) error { return set.Remove(u) })
}

func (s *PulledState) writeHeadOnly(u *url.URL, fn func(values.Set[*url.URL]) error) {
	id, ok := protocol.ParsePartitionUrl(s.partition)
	if !ok {
		return
	}
	batch := s.db.Begin(true)
	defer batch.Discard()
	err := fn(batch.SystemData(id).HeadOnly())
	if err == nil {
		err = batch.Commit()
	}
	if err != nil {
		s.log.Info("What is left to backfill could not be recorded; a restart may not backfill it",
			"account", u, "partition", s.partition, "error", err)
	}
}

// loadHeadOnly reads what an earlier process recorded as taken by its chain
// heads alone and did not backfill.
func (s *PulledState) loadHeadOnly() error {
	id, ok := protocol.ParsePartitionUrl(s.partition)
	if !ok {
		return nil
	}
	batch := s.db.Begin(false)
	defer batch.Discard()
	list, err := batch.SystemData(id).HeadOnly().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("read the accounts left to backfill: %w", err)
	}
	for _, u := range list {
		if s.headOnly == nil {
			s.headOnly = map[[32]byte]*url.URL{}
		}
		s.headOnly[accountKey(u)] = u
	}
	return nil
}

// backfillOne backfills one account and reports whether it holds all of it
// now (pull.Backfill).
func (s *PulledState) backfillOne(ctx context.Context, u *url.URL) bool {
	srcs, _, err := s.sourcesFor(ctx, u)
	if errors.Is(err, errNotThisPartition) {
		return true // Never this partition's to hold
	}
	if err != nil {
		return false
	}
	// Written to the database a page at a time (#4446).
	err = pull.Backfill(ctx, srcs, s.db, u, 0)
	if err != nil {
		s.log.Info("An account's entries could not be brought in yet", "account", u, "error", err)
		return false
	}
	if s.entire == nil {
		s.entire = map[[32]byte]bool{}
	}
	s.entire[accountKey(u)] = true
	return true
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
