// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	keys "gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The handoff is an overlap, not a switch (executor spec, "Sync", §3a; Paul,
// 2026-10-02; #4495). From the match, the node executes each block and takes
// the partition's block-ledger record of it: every account the record names
// must end in the state the record gives as of the block, and where this
// node's execution left it otherwise, the record's state stands. After
// OverlapBlocks blocks in a row with no difference, the node executes alone.
//
// A block is settled in two steps, both in the batch it executes in. Before
// its chains are anchored (settle): each account the record names, and each
// the block changed, is given the record's state but for its index chains,
// and the chains the record names are the chains the block changed -- so the
// root chain anchors, and the block's own record names, what the partition's
// did. Before its state tree is updated (verify): each such account's leaf
// must be the record's; the index chains, which the block's close writes, are
// the record's where they are not.
type overlap struct {
	partition *url.URL
	label     string
	log       *slog.Logger

	// peers is the partition's peers, in the order they are asked.
	peers func(ctx context.Context) []pull.Source

	// taken is told the accounts the overlap wrote by their chain heads:
	// they are backfilled (§2).
	taken func(...*url.URL)

	// ask bounds how long one block asks the peers for its record.
	ask time.Duration

	// blocks is how many clean blocks in a row end the overlap:
	// OverlapBlocks, unless a test sets it (SetOverlapBlocks).
	blocks int

	mu          sync.Mutex
	active      bool
	from        uint64 // the block the state was proven at
	last        uint64 // the last block settled
	clean       int    // blocks in a row with no difference
	checked     int    // blocks settled against their record
	frontier    int    // blocks no peer had produced yet
	differences int64
	endedAt     uint64

	// held is what settle took for the block being executed, for verify.
	// Only the goroutine executing blocks touches it.
	held *settling
}

// settling is a block between its two steps.
type settling struct {
	batch  *database.Batch
	n      uint64
	peer   string
	src    pull.Source
	states []*api.AccountState
	diffs  int
}

// OverlapBlocks is how many blocks in a row with no difference end the
// overlap. Eight: a handoff executes the groups it collected while it pulled
// before it reaches the frontier, and the run that measured it buffered eight
// (#4398, run 20260924T052134Z); those are the blocks a section loaded from
// the peers' current state can be ahead of. Eight blocks also hold at least
// two anchored blocks at rest, where a partition anchors every fourth.
const OverlapBlocks = 8

// overlapAsk bounds how long a block asks the peers for its record: each is
// asked once, and a peer that has not produced the block is not waited for.
const overlapAsk = 5 * time.Second

// mOverlapDifferences counts the accounts and chains the overlap found this
// node's execution had left other than the partition's record (#4495).
var mOverlapDifferences = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "accumulate",
	Subsystem: "join",
	Name:      "overlap_differences_total",
	Help:      "Accounts and chains a block executed during the overlap after a join left other than the partition's block-ledger record, which then stood",
}, []string{"partition"})

// start begins the overlap for the blocks after block, the block the state
// was proven at. A match that hands off is followed by them; one that does
// not is followed by another match, which begins it again.
func (o *overlap) start(block uint64) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.active = true
	o.from, o.last = block, block
	o.clean, o.checked, o.frontier = 0, 0, 0
	o.endedAt = 0
	o.log.Info("The overlap begins: each block is executed and settled against its record",
		"partition", o.partition, "proven", block, "until-clean", o.blocks)
}

// stop ends the overlap without its clean run: the node stopped executing.
func (o *overlap) stop() {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.active = false
}

// due reports whether block is one the overlap settles.
func (o *overlap) due(block uint64) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.active && block > o.from
}

// settle is a block's first step, after its transactions and before its
// chains are anchored (database.BlockSettler). It fails the block only when
// what it writes cannot be written: a record it cannot have is not a reason
// to hold a block, and the root watch is the backstop for such a block.
func (o *overlap) settle(batch *database.Batch, n uint64) error {
	if o == nil {
		return nil
	}
	o.held = nil
	if !o.due(n) {
		return nil
	}
	// Beside the join, not on its goroutine: a block is executing.
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), backfillKey{}, true), o.ask)
	defer cancel()

	records, src, ok := o.record(ctx, n)
	if !ok {
		// No peer has produced n: the node is at the frontier, where no
		// peer's state is ahead of the block, so nothing the block loaded
		// can be. It is executed alone.
		o.done(n, false, 0)
		return nil
	}

	var named, later []*url.URL
	var entries []*protocol.BlockEntry
	for _, r := range records {
		switch {
		case r == nil:
		case r.Index == n:
			entries = r.Entries
			named = append(named, api.RecordNames(r.Entries)...)
		case r.Index > n:
			later = append(later, api.RecordNames(r.Entries)...)
		}
	}
	for _, a := range batch.UpdatedAccounts() {
		named = append(named, a.Url())
	}
	named = userAccounts(named, nil)

	s := &settling{batch: batch, n: n, peer: sourceKey(src), src: src}
	if len(named) > 0 {
		states, err := statesAt(ctx, src, o.partition, named, n)
		if err != nil {
			o.log.Info("The states a block's record names could not be taken; the block is executed alone",
				"partition", o.partition, "block", n, "peer", s.peer, "error", err)
			o.done(n, false, 0)
			return nil
		}
		s.states = states
	}
	if err := o.settleNamed(s); err != nil {
		return err
	}
	if err := o.settleChains(s, entries); err != nil {
		return err
	}

	// The accounts later records name that sit in a section this block
	// loaded: a section loaded from a peer past n holds them as the blocks
	// after n left them, and they are put back as of n. Read after the named
	// accounts, whose reads may have loaded sections too.
	skip := map[[32]byte]bool{}
	for _, u := range named {
		skip[accountKey(u)] = true
	}
	var ahead []*url.URL
	for _, u := range userAccounts(later, skip) {
		if batch.LoadedSection(database.SectionOfAccount(u)) {
			ahead = append(ahead, u)
		}
	}
	if err := o.settleAhead(ctx, s, src, ahead); err != nil {
		return err
	}
	o.held = s
	return nil
}

// verify is a block's second step, after its last write to an account and
// before its state tree is updated (database.BlockSettler): each account the
// record names must hold the record's leaf. The index chains the block's
// close wrote are the record's where they are not -- a section loaded ahead
// of the block already held the entry the close appends -- and anything else
// that differs is said and left to the root watch.
func (o *overlap) verify(batch *database.Batch, n uint64) error {
	s := o.held
	o.held = nil
	if s == nil || s.batch != batch || s.n != n {
		return nil
	}
	var taken []*url.URL
	for _, st := range s.states {
		if st.NoLeaf || st.Main == nil {
			continue
		}
		local, _, err := executedLeaf(batch, st.Url)
		if err != nil {
			return errors.UnknownError.WithFormat("the leaf of %v: %w", st.Url, err)
		}
		if local == st.LeafHash {
			continue
		}
		if err := pull.SettleStates(batch, []*api.AccountState{st}, pull.SettleIndex); err != nil {
			return errors.UnknownError.WithFormat("write the record's index chains of %v: %w", st.Url, err)
		}
		after, _, err := executedLeaf(batch, st.Url)
		if err != nil {
			return errors.UnknownError.WithFormat("the leaf of %v: %w", st.Url, err)
		}
		why := "its index chains were not the record's"
		if after != st.LeafHash {
			why = "it is not the record's after its index chains; left to the root watch"
		}
		s.diffs++
		o.said(n, st.Url, local, st.LeafHash, s.peer, why)
		taken = append(taken, st.Url)
	}
	if len(taken) > 0 && o.taken != nil {
		o.taken(taken...)
	}
	if err := o.verifySystem(s); err != nil {
		return err
	}
	o.done(n, true, s.diffs)
	return nil
}

// verifySystem gives the partition's own system accounts the record's state
// where the block left them otherwise. They are written by the block's close,
// so they are compared only here, and every one is, whether the record names
// it or not: the synthetic ledger never is (ChangedAccounts). A transaction
// this node refused because a section it loaded already held its effects
// queued nothing, where the peers' execution queued its local deliveries; the
// queue is the record's, and the messages behind it are taken
// (KeepQueuedMessages). The partition ledger is not compared: a peer past the
// block does not retain its scheduled events, and its chains -- what the block
// anchored -- are the root watch's.
func (o *overlap) verifySystem(s *settling) error {
	var urls []*url.URL
	for _, u := range SystemAccounts(o.partition) {
		if !u.Equal(o.partition.JoinPath(protocol.Ledger)) {
			urls = append(urls, u)
		}
	}
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), backfillKey{}, true), o.ask)
	defer cancel()
	states, err := statesAt(ctx, s.src, o.partition, urls, s.n)
	if err != nil {
		o.log.Info("The system accounts could not be taken as of a block; the root watch decides",
			"partition", o.partition, "block", s.n, "peer", s.peer, "error", err)
		return nil
	}
	for _, st := range states {
		if st.NoLeaf || st.Main == nil {
			continue
		}
		local, _, err := executedLeaf(s.batch, st.Url)
		if err != nil {
			return errors.UnknownError.WithFormat("the leaf of %v: %w", st.Url, err)
		}
		if local == st.LeafHash {
			continue
		}
		if err := pull.SettleStates(s.batch, []*api.AccountState{st}, pull.SettleAll); err != nil {
			return errors.UnknownError.WithFormat("write the record's state of %v: %w", st.Url, err)
		}
		if err := pull.KeepQueuedMessages(ctx, s.src, s.batch, st); err != nil {
			return errors.UnknownError.WithFormat("the queued deliveries of %v: %w", st.Url, err)
		}
		after, _, err := executedLeaf(s.batch, st.Url)
		if err != nil {
			return errors.UnknownError.WithFormat("the leaf of %v: %w", st.Url, err)
		}
		why := "a system account"
		if after != st.LeafHash {
			why = "a system account, not the record's after it was written; left to the root watch"
		}
		s.diffs++
		o.said(s.n, st.Url, local, st.LeafHash, s.peer, why)
	}
	return nil
}

// record asks the peers in turn, once each, for the block-ledger records from
// n on. A peer that has not produced n says so at once and is not waited for.
func (o *overlap) record(ctx context.Context, n uint64) ([]*api.BlockLedgerRecord, pull.Source, bool) {
	for _, src := range o.peers(ctx) {
		if ctx.Err() != nil {
			break
		}
		rec, err := src.QueryChangedAccountStates(ctx, o.partition, &api.ChangedAccountStatesQuery{After: n - 1})
		if err != nil || rec == nil || rec.Through < n {
			continue
		}
		return rec.Records, src, true
	}
	return nil, nil, false
}

// userAccounts is the accounts once each, without the partition's own system
// accounts and those in skip. A system account is written after the block's
// transactions, by the block's close, and its section is never forgotten, so
// no load is ahead of it (§3a); its agreement is the root watch's.
func userAccounts(urls []*url.URL, skip map[[32]byte]bool) []*url.URL {
	seen := map[[32]byte]bool{}
	var out []*url.URL
	for _, u := range urls {
		if !Routable(u) {
			continue
		}
		if _, ok := protocol.ParsePartitionUrl(u); ok {
			continue
		}
		k := accountKey(u)
		if seen[k] || skip[k] {
			continue
		}
		seen[k] = true
		out = append(out, u.StripExtras())
	}
	return out
}

// settleNamed gives each account the record names, and each the block
// changed, the record's state but for its index chains, where it differs.
func (o *overlap) settleNamed(s *settling) error {
	var taken []*url.URL
	for _, st := range s.states {
		u := st.Url
		local, differs, exists, err := preCloseLeaf(s.batch, st)
		if err != nil {
			return errors.UnknownError.WithFormat("the leaf of %v: %w", u, err)
		}
		switch {
		case st.NoLeaf && !exists:
			continue
		case st.NoLeaf:
			// This execution made an account the peers do not hold at n.
			// It is not removed here; the root watch repairs it ("Two
			// mismatches", 2).
			s.diffs++
			o.said(s.n, u, local, [32]byte{}, s.peer, "the record holds no such account; left to the root watch")
			continue
		case st.Main == nil:
			continue // Not retained by the peer as of n: not compared
		case exists && !differs:
			continue
		}
		if err := pull.SettleStates(s.batch, []*api.AccountState{st}, pull.SettleNotIndex); err != nil {
			return errors.UnknownError.WithFormat("write the record's state of %v: %w", u, err)
		}
		s.diffs++
		o.said(s.n, u, local, st.LeafHash, s.peer, "")
		taken = append(taken, u)
	}
	if len(taken) > 0 && o.taken != nil {
		o.taken(taken...)
	}
	return nil
}

// preCloseLeaf compares an account as the block's transactions left it with
// the record's state, but for the index chains, which the block's close has
// not written yet: the account's leaf with its index chains taken from the
// record, in a batch that is discarded. It returns the account's own leaf,
// whether the two differ, and whether the account exists.
func preCloseLeaf(batch *database.Batch, st *api.AccountState) ([32]byte, bool, bool, error) {
	local, exists, err := executedLeaf(batch, st.Url)
	if err != nil || !exists || st.NoLeaf || st.Main == nil {
		return local, exists, exists, err
	}
	scratch := batch.Begin(true)
	defer scratch.Discard()
	if err := pull.SettleStates(scratch, []*api.AccountState{st}, pull.SettleIndex); err != nil {
		return local, false, true, err
	}
	h, _, err := executedLeaf(scratch, st.Url)
	return local, h != st.LeafHash, true, err
}

// settleChains makes the chains this block changed the chains the record of n
// names, so that the block anchors them and its own record names them as the
// partition's does. A chain the record names that this execution did not
// change -- a transaction the peers executed and this node refused, because a
// section it loaded already held its effects -- is marked changed: its head is
// the record's, written by settleNamed or already held. A chain this execution
// changed that the record does not name cannot be unmarked; it is said, and
// the root watch is the backstop for it.
func (o *overlap) settleChains(s *settling, entries []*protocol.BlockEntry) error {
	want := map[string]*protocol.BlockEntry{}
	for _, e := range entries {
		if e == nil || e.Account == nil || e.Chain == "" || !Routable(e.Account) {
			continue
		}
		if _, ok := protocol.ParsePartitionUrl(e.Account); ok {
			continue
		}
		want[chainKey(e.Account, e.Chain)] = e
	}
	have := map[string]bool{}
	for _, a := range s.batch.UpdatedAccounts() {
		if _, ok := protocol.ParsePartitionUrl(a.Url()); ok {
			continue
		}
		list, err := a.UpdatedChains()
		if err != nil {
			return errors.UnknownError.WithFormat("the chains %v changed: %w", a.Url(), err)
		}
		for _, e := range list {
			have[chainKey(a.Url(), e.Chain)] = true
		}
	}

	for k, e := range want {
		if have[k] {
			continue
		}
		chain, err := s.batch.Account(e.Account).ChainByName(e.Chain)
		if err != nil {
			return errors.UnknownError.WithFormat("chain %v#%s: %w", e.Account, e.Chain, err)
		}
		head, err := chain.Inner().Head().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("the head of %v#%s: %w", e.Account, e.Chain, err)
		}
		// Written again, it is a chain this block changed.
		if err := chain.Inner().Head().Put(head.Copy()); err != nil {
			return errors.UnknownError.WithFormat("mark %v#%s changed: %w", e.Account, e.Chain, err)
		}
		s.diffs++
		o.chainSaid(s.n, k, "the record names a change this execution did not make")
	}
	for k := range have {
		if _, ok := want[k]; !ok {
			s.diffs++
			o.chainSaid(s.n, k, "this execution changed a chain the record does not name; left to the root watch")
		}
	}
	return nil
}

// chainKey names an account's chain.
func chainKey(u *url.URL, chain string) string {
	return strings.ToLower(u.StripExtras().String()) + "#" + strings.ToLower(chain)
}

// settleAhead puts back, as of n, the accounts in sections this block loaded
// that later records name and n's does not: they are written beneath the
// block's records, as a section's load writes them, and their leaves set in
// the block's tree, so the block's record does not name them.
func (o *overlap) settleAhead(ctx context.Context, s *settling, src pull.Source, urls []*url.URL) error {
	if len(urls) == 0 {
		return nil
	}
	states, err := statesAt(ctx, src, o.partition, urls, s.n)
	if err != nil {
		o.log.Info("The states of a section a block loaded could not be taken as of the block; the root watch decides",
			"partition", o.partition, "block", s.n, "peer", s.peer, "error", err)
		return nil
	}
	tree := s.batch.BPT()
	var taken []*url.URL
	for _, st := range states {
		u := st.Url
		key := keys.NewKey("Account", u)
		var local [32]byte
		v, err := tree.Get(key)
		switch {
		case err == nil && len(v) == 32:
			local = [32]byte(v)
		case err == nil, errors.Is(err, errors.NotFound):
		default:
			return errors.UnknownError.WithFormat("the leaf of %v: %w", u, err)
		}
		switch {
		case st.NoLeaf && local == [32]byte{}:
			continue
		case st.NoLeaf:
			// Made after n by the peers: not there at n.
			err := s.batch.WriteBeneath(func(c *database.Batch) error { return c.ForgetAccount(u) })
			if err == nil {
				err = tree.Delete(key)
			}
			if err != nil && !errors.Is(err, errors.NotFound) {
				return errors.UnknownError.WithFormat("forget %v: %w", u, err)
			}
		case st.Main == nil, local == st.LeafHash:
			continue
		default:
			err := s.batch.WriteBeneath(func(c *database.Batch) error { return pull.WriteStates(c, []*api.AccountState{st}) })
			if err == nil {
				err = tree.Insert(key, st.LeafHash[:])
			}
			if err != nil {
				return errors.UnknownError.WithFormat("write the state of %v as of %d: %w", u, s.n, err)
			}
			taken = append(taken, u)
		}
		s.diffs++
		o.said(s.n, u, local, st.LeafHash, s.peer, "a section the block loaded held it ahead of the block")
	}
	if len(taken) > 0 && o.taken != nil {
		o.taken(taken...)
	}
	return nil
}

// executedLeaf is the leaf the batch holds u with, and whether u exists.
func executedLeaf(batch *database.Batch, u *url.URL) ([32]byte, bool, error) {
	a := batch.Account(u)
	_, err := a.Main().Get()
	switch {
	case err == nil:
	case errors.Is(err, errors.NotFound):
		return [32]byte{}, false, nil
	default:
		return [32]byte{}, false, err
	}
	h, err := a.Hash()
	return h, true, err
}

// said counts an account a block left other than its record, and says it:
// once per account and step of a block.
func (o *overlap) said(n uint64, u *url.URL, local, record [32]byte, peer, why string) {
	o.count()
	args := []any{"partition", o.partition, "block", n, "account", u,
		"local", fmt.Sprintf("%x", local[:8]), "record", fmt.Sprintf("%x", record[:8]), "peer", peer}
	if why != "" {
		args = append(args, "why", why)
	}
	o.log.Warn("A block left an account other than its record; the record's state stands", args...)
}

// chainSaid counts a chain a block changed other than its record says, and
// says it.
func (o *overlap) chainSaid(n uint64, chain, why string) {
	o.count()
	o.log.Warn("A block changed a chain other than its record; the record stands",
		"partition", o.partition, "block", n, "chain", chain, "why", why)
}

func (o *overlap) count() {
	mOverlapDifferences.WithLabelValues(o.label).Inc()
	o.mu.Lock()
	o.differences++
	o.mu.Unlock()
}

// done records how block n settled, and ends the overlap after OverlapBlocks
// blocks in a row with no difference. A block settled again -- one that was
// discarded and executed again -- is not counted twice.
func (o *overlap) done(n uint64, checked bool, diffs int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.active || n <= o.last {
		return
	}
	o.last = n
	if checked {
		o.checked++
	} else {
		o.frontier++
	}
	if diffs > 0 {
		o.clean = 0
		return
	}
	o.clean++
	if o.clean < o.blocks {
		return
	}
	o.active = false
	o.endedAt = n
	o.log.Info("The overlap is over: the node executes alone",
		"partition", o.partition, "block", n, "proven", o.from, "checked", o.checked, "frontier", o.frontier, "differences", o.differences)
}

// OverlapStatus is what the overlap has done since the last match.
type OverlapStatus struct {
	Active      bool
	From        uint64 // the block the overlap began after: the last match
	EndedAt     uint64 // the block the overlap ended at; zero while it runs
	Checked     int    // blocks settled against their record
	Frontier    int    // blocks no peer had produced when they were settled
	Differences int64  // accounts and chains a record settled, since the process started
}

// SetOverlapBlocks sets how many clean blocks in a row end the overlap, for a
// test that needs it over sooner or later than OverlapBlocks.
func (s *PulledState) SetOverlapBlocks(k int) {
	s.overlap.mu.Lock()
	defer s.overlap.mu.Unlock()
	s.overlap.blocks = k
}

// Overlap is what the overlap after the last match has done (#4495).
func (s *PulledState) Overlap() OverlapStatus {
	o := s.overlap
	o.mu.Lock()
	defer o.mu.Unlock()
	return OverlapStatus{Active: o.active, From: o.from, EndedAt: o.endedAt, Checked: o.checked, Frontier: o.frontier, Differences: o.differences}
}

// SettleBlock implements [database.BlockSettler].
func (x *sections) SettleBlock(batch *database.Batch, block uint64) error {
	if x == nil || x.overlap == nil {
		return nil
	}
	return x.overlap.settle(batch, block)
}

// VerifyBlock implements [database.BlockSettler].
func (x *sections) VerifyBlock(batch *database.Batch, block uint64) error {
	if x == nil || x.overlap == nil {
		return nil
	}
	return x.overlap.verify(batch, block)
}
