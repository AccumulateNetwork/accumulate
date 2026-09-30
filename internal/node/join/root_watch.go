// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/tracker"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
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
// The state handed off at is proven: the join hands off only on a match at the
// block its state is complete at (Matched; #4442). From here executed is the
// last block compared, or the block handed off at.
func (s *PulledState) HandedOff(q uint64) {
	s.executed = q
	s.checked.Store(q)
	// The store is a block's state again: the pull that began is done.
	if err := s.writePullStarted(0); err != nil {
		s.log.Info("The end of the pull could not be recorded", "partition", s.partition, "error", err)
	}
	if s.sync != nil {
		s.served = s.sync.servedBy
	}
	s.sync = nil
	s.repairFrom = 0
}

// Diverged compares the root this node computed after each block it executed
// since the handoff with the partition's signed anchor for that block, at
// every block that sent one (executor spec, "Sync", "Two
// mismatches"). A mismatch is reported, and the next pull
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
			s.locateExecuted(ctx, batch, o.Block, local)
			// Since the last comparison: the last block whose root was
			// compared and matched, or the block the node handed off at
			// (executor spec, "Sync", "Two mismatches").
			// A repair that does not bring a match is followed by one that
			// walks the tree again (startRepair).
			s.RepairFrom(s.executed)
			return o.Block, true, nil
		}
		s.executed = o.Block
		s.checked.Store(o.Block)
		s.provenAt = o.Block
		s.repairs = 0
		s.served = nil // what it took is proven
		proven = true
		if s.ownRoot {
			// ACTIVE on its own root since the whole-restart exit, and now
			// a signed anchor has proved a block it executed: the only
			// line that says a resumed node is in agreement (#4483).
			s.ownRoot = false
			s.agreedAt.Store(o.Block)
			s.log.Info("This node is executing in agreement; its own root matched its signed anchor after the whole-restart exit",
				"block", o.Block, "partition", s.partition)
		}
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

// backfillPause is how long the backfill waits before it asks again for the
// accounts it could not bring in, and how often it looks whether the node is
// ACTIVE yet.
const backfillPause = 2 * time.Second

// Backfill implements [State]: it brings in the entries below the open mark
// set of the accounts the join took by their chain heads alone
// (pull.Backfill), while the node executes, until every one is held whole or
// ctx ends. It writes only below each chain's head, so the blocks the node
// executes do not race it.
//
// It runs beside the root watch, never inside it: one account of the
// Directory is minutes of pages, and a root watch waiting on it compares no
// block's root for as long (#4472). It does nothing while the node is not
// ACTIVE: a pull, a repair or a demotion rewrites the very accounts it
// writes, so the join ends it before it demotes and pulls, and starts it
// again after the next handoff (Run).
func (s *PulledState) Backfill(ctx context.Context) {
	ctx = context.WithValue(ctx, backfillKey{}, true)
	for ctx.Err() == nil {
		if s.machine.State() != nodestate.StateActive {
			if !pause(ctx, backfillPause) {
				return
			}
			continue
		}
		left := s.headOnlyList()
		if len(left) == 0 {
			return
		}
		for _, u := range left {
			if ctx.Err() != nil || s.machine.State() != nodestate.StateActive {
				break
			}
			if s.backfillOne(ctx, u) {
				s.backfillDone(u)
			}
		}
		if s.BackfillLeft() == 0 {
			return
		}
		// What is left could not be brought in on this pass.
		if !pause(ctx, backfillPause) {
			return
		}
	}
}

type backfillKey struct{}

// Backfilling is whether ctx is the backfill's. The backfill runs beside the
// join, not on its goroutine: what the join's sources do between two of the
// join's own reads, such as the simulator's pull hook, must not run from it.
func Backfilling(ctx context.Context) bool {
	b, _ := ctx.Value(backfillKey{}).(bool)
	return b
}

// pause waits d, and reports false if ctx ended first.
func pause(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// BackfillLeft is how many accounts taken by their chain heads alone the
// node does not hold whole yet.
func (s *PulledState) BackfillLeft() int {
	s.backfillMu.Lock()
	defer s.backfillMu.Unlock()
	return len(s.headOnly)
}

// headOnlyList is the accounts left to backfill, in a stable order.
func (s *PulledState) headOnlyList() []*url.URL {
	s.backfillMu.Lock()
	defer s.backfillMu.Unlock()
	out := make([]*url.URL, 0, len(s.headOnly))
	for _, u := range s.headOnly {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// backfillDone records that the backfill brought u in whole, and says so.
func (s *PulledState) backfillDone(u *url.URL) {
	s.backfillMu.Lock()
	defer s.backfillMu.Unlock()
	s.unmarkHeadOnlyLocked(accountKey(u))
	s.backfilled++
	s.sayBackfillLocked()
}

// sayBackfillLocked reports how much history is left to bring in: always on
// the gauge, in the log at most once a minute, and once when the last account
// is held whole (#4471). The caller holds backfillMu.
func (s *PulledState) sayBackfillLocked() {
	left := len(s.headOnly)
	mBackfillLeft.WithLabelValues(s.label).Set(float64(left))
	if left == 0 {
		s.log.Info("The backfill is done: every account is held whole",
			"partition", s.partition, "backfilled", s.backfilled, "executed", s.checked.Load())
		return
	}
	if time.Since(s.backfillSaid) < stallSayEvery {
		return
	}
	s.backfillSaid = time.Now()
	s.log.Info("Backfilling the history of accounts taken by their chain heads",
		"partition", s.partition, "backfilled", s.backfilled, "left", left, "executed", s.checked.Load())
}

// isEntire is whether this node holds u whole (backfillOne).
func (s *PulledState) isEntire(u *url.URL) bool {
	s.backfillMu.Lock()
	defer s.backfillMu.Unlock()
	return s.entire[accountKey(u)]
}

// markHeadOnly records that u was taken by its chain heads alone: in memory,
// and under SystemData, where no pull reaches, so that a restart before the
// backfill ends still backfills it. The next pull would not name it again:
// its leaf is the peers'.
//
// The accounts of one answer are recorded in one commit (#4461).
func (s *PulledState) markHeadOnly(urls ...*url.URL) {
	s.backfillMu.Lock()
	defer s.backfillMu.Unlock()
	var add []*url.URL
	for _, u := range urls {
		k := accountKey(u)
		if _, ok := s.headOnly[k]; ok {
			continue
		}
		if s.headOnly == nil {
			s.headOnly = map[[32]byte]*url.URL{}
		}
		s.headOnly[k] = u
		add = append(add, u)
	}
	if len(add) == 0 {
		return
	}
	s.writeHeadOnly(add[0], func(set values.Set[*url.URL]) error { return set.Add(add...) })
	mBackfillLeft.WithLabelValues(s.label).Set(float64(len(s.headOnly)))
}

// unmarkHeadOnly records that the account keyed k is held whole.
func (s *PulledState) unmarkHeadOnly(k [32]byte) {
	s.backfillMu.Lock()
	defer s.backfillMu.Unlock()
	s.unmarkHeadOnlyLocked(k)
	mBackfillLeft.WithLabelValues(s.label).Set(float64(len(s.headOnly)))
}

// unmarkHeadOnlyLocked is unmarkHeadOnly with backfillMu held.
func (s *PulledState) unmarkHeadOnlyLocked(k [32]byte) {
	u, ok := s.headOnly[k]
	if !ok {
		return
	}
	delete(s.headOnly, k)
	s.writeHeadOnly(u, func(set values.Set[*url.URL]) error { return set.Remove(u) })
}

// writeHeadOnly is called with backfillMu held, so that the record and the
// set in memory move together.
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
// now (pull.Backfill). An account that takes longer than a minute says how
// far it has got, at most once a minute (#4472).
func (s *PulledState) backfillOne(ctx context.Context, u *url.URL) bool {
	srcs, _, err := s.sourcesFor(ctx, u)
	if errors.Is(err, errNotThisPartition) {
		return true // Never this partition's to hold
	}
	if err != nil {
		return false
	}
	// Written to the database a page at a time (#4446).
	err = pull.Backfill(ctx, srcs, s.db, u, 0, s.backfillProgress(u))
	if err != nil {
		s.log.Info("An account's entries could not be brought in yet", "account", u, "error", err)
		return false
	}
	s.backfillMu.Lock()
	defer s.backfillMu.Unlock()
	if s.entire == nil {
		s.entire = map[[32]byte]bool{}
	}
	s.entire[accountKey(u)] = true
	return true
}

// backfillProgress is what one account's backfill says of itself: nothing in
// its first minute, then the chain, the entries written and the chain's
// height, at most once a minute.
func (s *PulledState) backfillProgress(u *url.URL) func(chain string, done, total uint64) {
	start := s.clock()
	said := start
	return func(chain string, done, total uint64) {
		now := s.clock()
		if now.Sub(said) < stallSayEvery {
			return
		}
		said = now
		s.log.Info("Backfilling a large account",
			"partition", s.partition, "account", u, "chain", chain, "entries", done, "of", total,
			"for", now.Sub(start).Round(time.Second))
	}
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
	// From the newest entry: the blocks compared are the ones executed since
	// the handoff, and a node that joined holds the chain's recent window,
	// not its first entry (#4452).
	_, entry, err := indexing.SearchIndexChain2(index, uint64(head.Count-1), indexing.MatchAfter, indexing.SearchIndexChainByBlock(n+1))
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

// locateExecuted names the accounts whose leaves differ, after block n, between
// this node's tree and the peers' (#4441): the tree as of n from the history
// this node retains, against a peer's as of n. It is how an execution that
// differs from the peers' is traced to what differed; it repairs nothing
// (the repair from the block ledger does). A node that retains no history for
// n says so.
func (s *PulledState) locateExecuted(ctx context.Context, batch *database.Batch, n uint64, root [32]byte) {
	tree := treeAt{tree: batch.BPT(), height: n, root: root}
	diffs, peer, err := locateAt(ctx, s.partition, s.partitionSources(ctx), tree, n)
	s.mismatchAt, s.mismatches = n, diffs
	if err != nil {
		s.log.Info("The accounts an executed block's root differs in could not be located",
			"partition", s.partition, "block", n, "error", err)
		return
	}
	for i, d := range diffs {
		s.log.Warn("An executed block left an account other than the peer's",
			"partition", s.partition, "block", n, "peer", peer, "account", d.Account,
			"key", fmt.Sprintf("%x", d.KeyHash), "local", fmt.Sprintf("%x", d.Local), "peer-leaf", fmt.Sprintf("%x", d.Peer))
		if i < maxStatesSaid && d.Account != nil {
			s.sayStates(ctx, batch, d.Account)
		}
	}
}

// maxStatesSaid is how many differing accounts a miss logs the states of.
const maxStatesSaid = 4

// sayStates logs an account's main state as this node holds it and as each
// peer serves it now, so a miss says which field differs. Both are the
// current state: the node has executed past the missed block by a block or
// two and the peer by as many, so a field that moves every block differs
// for that reason alone; one that does not is the difference.
func (s *PulledState) sayStates(ctx context.Context, batch *database.Batch, u *url.URL) {
	local, err := batch.Account(u).Main().Get()
	localJSON := "<none>"
	if err == nil {
		if b, err := json.Marshal(local); err == nil {
			localJSON = string(b)
		}
	}
	peers := map[string]string{}
	for _, src := range s.partitionSources(ctx) {
		rec, err := src.QueryAccount(ctx, u, nil)
		if err != nil || rec == nil || rec.Account == nil {
			continue
		}
		if b, err := json.Marshal(rec.Account); err == nil {
			peers[sourceKey(src)] = string(b)
		}
		if len(peers) >= 2 {
			break
		}
	}
	s.log.Warn("The states of an account an executed block left other than the peer's",
		"partition", s.partition, "account", u, "local", localJSON, "peers", peers)
	s.sayMainChain(ctx, batch, u)
}

// mainEntriesSaid is how many of an account's newest main chain entries a
// miss compares.
const mainEntriesSaid = 8

// sayMainChain logs the newest entries of an account's main chain as this
// node holds them, and for each, where the peers hold it: its index on their
// chain and the block they first received it at, or that they have not
// executed it. An entry this node executed that its peers executed later, or
// not at all, is the transaction the miss is (#4442).
func (s *PulledState) sayMainChain(ctx context.Context, batch *database.Batch, u *url.URL) {
	chain := batch.Account(u).MainChain()
	head, err := chain.Head().Get()
	if err != nil || head.Count == 0 {
		return
	}
	srcs := s.partitionSources(ctx)
	if len(srcs) == 0 {
		return
	}
	src := srcs[0]
	from := max(head.Count-mainEntriesSaid, 0)
	for i := from; i < head.Count; i++ {
		e, err := chain.Inner().Entry(i)
		if err != nil {
			continue
		}
		id := u.WithTxID([32]byte(e))
		peerIndex, peerReceived := "absent", uint64(0)
		if r, err := src.QueryChainEntry(ctx, u, &api.ChainQuery{Name: "main", Entry: e}); err == nil && r != nil {
			peerIndex = fmt.Sprint(r.Index)
		}
		if m, err := src.QueryMessage(ctx, id, nil); err == nil && m != nil {
			peerReceived = m.Received
		}
		s.log.Warn("A main chain entry of an account an executed block left other than the peer's",
			"partition", s.partition, "account", u, "index", i, "txid", id,
			"peer", sourceKey(src), "peer-index", peerIndex, "peer-received", peerReceived)
	}
}
