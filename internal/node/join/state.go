// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"log/slog"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/enumerate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/tracker"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// What state this node is in, as a number an operator can watch: 0 booting,
// 1 waiting, 2 active, 3 complete. A node stuck joining is invisible without
// it — the refusal counter only moves if somebody asks (#4295).
var mNodeState = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "accumulate",
	Subsystem: "node",
	Name:      "state",
	Help:      "This node's state for the partition: 0 booting, 1 waiting, 2 active, 3 complete (executor spec, \"Sync\", step 5)",
}, []string{"partition"})

func stateNumber(s nodestate.State) float64 {
	switch s {
	case nodestate.StateWaiting:
		return 1
	case nodestate.StateActive:
		return 2
	case nodestate.StateComplete:
		return 3
	default:
		return 0
	}
}

const (
	// pullChunk is how many accounts are fetched before the round settles
	// them. Every fetched account holds an open child batch until it settles
	// (pull.MaxHeld), so the chunk bounds what a round holds in memory.
	pullChunk = 256

	// maxSettleRounds is how many rounds a fetched account is held waiting for
	// the Directory to anchor the block it was served at, before it is thrown
	// away and asked for again.
	//
	// It is bounded for two reasons: a held account keeps its state in memory
	// (pull.MaxHeld) and its batch keeps a version of the store open (#4279).
	// The anchor for a block arrives a few blocks later, so a handful of
	// rounds is the whole of the wait; a peer claiming a block that will never
	// be anchored must not stop the join for everyone.
	maxSettleRounds = 4

	// staleEvery is how often the BPT page diff runs regardless of what the
	// block ledger named. It is the backstop, and a backstop that only runs
	// when the primary named nothing is not one (#4306).
	staleEvery = 8
)

// PulledState is the join's state half (#4293): it pulls what the node lacks
// from running peers, verified against the root the Directory anchored for the
// block the peer served it at, and says when the local root equals one of
// those roots — the block the node then executes from (executor spec, "Sync",
// steps 3 and 4).
//
// What it pulls is what the BLOCK LEDGER says the blocks changed, read from a
// peer: every block records (account, chain, index) for every chain it
// changed, and the state root commits to that record. A block's envelopes name
// principals, signers and anchor pools — never the system accounts every block
// changes, and sometimes a name that cannot be routed — so a tree built from
// them chases a root it can never reach (#4306). The BPT page diff is the
// backstop, on a cadence and whenever the ledger walk cannot cover the span.
//
// Every read is addressed at a NAMED PEER. The node's own routed client
// answers from the node's own store for any service it provides, which for a
// joining node is the un-executed store the pull exists to fill (#4303).
type PulledState struct {
	partition *url.URL
	db        *database.Database
	sources   Sources
	anchors   *pull.DirectoryAnchors
	machine   *nodestate.Machine
	tracker   *tracker.Tracker
	log       *slog.Logger

	spine bool   // the Directory's spine has been pulled
	round uint64 // how many rounds have run, for the backstop's cadence
	wide  bool   // the last ledger walk could not cover (R, Q]

	// held is what has been fetched and not yet settled: state a peer served
	// at a block the Directory has not anchored yet. It is HELD rather than
	// thrown away, because a re-fetch next round is served at a newer block
	// the Directory has not anchored either -- which is a treadmill, and is
	// why 2,171 pull rounds in run 20260918T131713Z all ended pulled=0.
	held []*heldBatch

	// refused are the accounts a round could not pull — served at a block the
	// Directory has not anchored yet, or served by a peer that could not be
	// verified. They are asked for again next round: an account dropped once
	// is an account the local root can never account for, and the join would
	// wait for a match that cannot come.
	refused []*url.URL
}

// StateOptions are what the state half needs.
type StateOptions struct {
	// Partition is this node's partition.
	Partition *url.URL

	// Database is this node's store.
	Database *database.Database

	// Sources finds the peers to pull from. It is not a querier: a querier
	// this node holds answers from this node, and a joining node answering
	// itself is #4303.
	Sources Sources

	Logger *slog.Logger
}

// NewState wires the pull, the anchor reader and the tracker together.
func NewState(opts StateOptions) (*PulledState, error) {
	if opts.Partition == nil || opts.Database == nil || opts.Sources == nil {
		return nil, errors.BadRequest.With("a join's state needs a partition, a database and peers to pull from")
	}
	machine := nodestate.New(opts.Partition)
	track, err := tracker.New(opts.Database, machine)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	s := &PulledState{
		partition: opts.Partition,
		db:        opts.Database,
		sources:   opts.Sources,
		machine:   machine,
		tracker:   track,
		log:       log.With("module", "join"),
	}
	s.anchors = &pull.DirectoryAnchors{
		// The Directory's anchors are what everything else is verified
		// against, so they are read from a peer that has executed them —
		// never from this node, whose anchor pool is the one the pull has not
		// filled yet (#4303).
		Query:    opts.Sources.Querier(protocol.DnUrl()),
		OnAnchor: track.Observe,
	}

	// Watchable from the moment the node starts joining, and on every change.
	label := opts.Partition.String()
	if id, ok := protocol.ParsePartitionUrl(opts.Partition); ok {
		label = strings.ToLower(id)
	}
	mNodeState.WithLabelValues(label).Set(stateNumber(machine.State()))
	machine.OnChange(func(ad nodestate.Advertisement) {
		mNodeState.WithLabelValues(label).Set(stateNumber(ad.State))
	})
	return s, nil
}

// Machine is the node's state — BOOTING until the root matches, ACTIVE after.
// The node's own services are given it, and refuse what they cannot answer
// while it is joining (#4295). It is handed over rather than registered: a
// process can run several nodes of one partition, and each has its own.
func (s *PulledState) Machine() *nodestate.Machine { return s.machine }

// Pull fetches what the blocks changed, and the Directory's spine the first
// time. Every account is verified against the root the Directory anchored for
// the block it was served at; one that cannot be verified is not written, and
// is asked for again next round.
func (s *PulledState) Pull(ctx context.Context) error {
	s.round++

	// The anchors first: they are what everything else is verified against,
	// and reading them is what tells the tracker which roots to watch for.
	err := s.anchors.Read(ctx)
	if err != nil {
		return errors.UnknownError.WithFormat("read the Directory's anchors: %w", err)
	}

	if !s.spine {
		err := s.pullSpine(ctx)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
		s.spine = true
	}

	// What earlier rounds fetched and could not verify yet is settled first,
	// against the block it was served at.
	pulled, refused := s.settleHeld(ctx)

	// What could not be pulled last round is asked for again, with whatever
	// this round's blocks changed.
	accounts := append(refused, s.refused...)
	s.refused = nil

	changed, err := s.changedAccounts(ctx)
	if err != nil {
		// A peer that cannot serve its block ledger this second is not a
		// reason to abandon the join: the backstop below covers the round.
		s.log.Info("The block ledger could not be read this round", "partition", s.partition, "error", err)
	}
	accounts = append(accounts, changed...)

	// The page diff is the backstop, and it runs on its own cadence rather
	// than only when the ledger named nothing. Gating it on an empty set made
	// it unreachable: one name that can never be satisfied keeps the set
	// non-empty for the life of the process (#4306).
	// The first round scans too: a node that has just started does not know
	// whether the store it holds is the state of block R -- a fresh node's is
	// genesis and the network is at half a million -- so the walk alone can
	// leave it missing everything it never changed.
	if s.round == 1 || len(changed) == 0 || s.wide || s.round%staleEvery == 0 {
		stale, err := s.staleAccounts(ctx)
		if err != nil {
			s.log.Info("The peer's BPT could not be paged this round", "partition", s.partition, "error", err)
		}
		accounts = append(accounts, stale...)
	}

	accounts = dedupe(accounts)
	if len(accounts) == 0 {
		if pulled > 0 {
			s.log.Info("Pulled the accounts held from an earlier round",
				"partition", s.partition, "pulled", pulled)
		}
		return nil
	}

	got, missed := s.fetch(ctx, accounts)
	pulled += got
	s.refused = missed
	s.log.Info("Pulled the accounts the block ledger named", "partition", s.partition,
		"asked", len(accounts), "pulled", pulled, "refused", len(missed), "held", s.heldCount())
	return nil
}

// heldCount is how many fetched accounts are waiting for their anchor.
func (s *PulledState) heldCount() int {
	n := 0
	for _, h := range s.held {
		n += len(h.accounts)
	}
	return n
}

// changedAccounts is the union of what the blocks in (R, Q] changed, where R
// is the block this node's state is and Q is the block a peer's is: the block
// ledger's answer, not the envelopes' (executor spec, "Sync", step 3).
func (s *PulledState) changedAccounts(ctx context.Context) ([]*url.URL, error) {
	s.wide = false

	r, err := s.localBlock()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	q := api.Querier2{Querier: s.sources.Querier(s.partition)}
	peer, err := s.peerBlock(ctx, q)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if peer <= r {
		// This node's state is at or past what the peer has executed. There
		// is nothing for the walk to say; the backstop decides the round.
		return nil, nil
	}

	if peer-r > MaxLedgerSpan {
		// Further behind than a walk is worth, and the walk would be wasted:
		// the page diff runs instead and answers the same question in one
		// scan. That is the case of a node joining from genesis.
		s.wide = true
		return nil, nil
	}

	entries, err := blockLedger(ctx, q, s.partition, r, peer)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return ChangedAccounts(s.partition, entries), nil
}

// localBlock is the block this node's state is: what its own ledger says. A
// node with no ledger at all has executed nothing.
func (s *PulledState) localBlock() (uint64, error) {
	batch := s.db.Begin(false)
	defer batch.Discard()
	var ledger *protocol.SystemLedger
	err := batch.Account(s.partition.JoinPath(protocol.Ledger)).Main().GetAs(&ledger)
	switch {
	case err == nil:
		return ledger.Index, nil
	case errors.Is(err, errors.NotFound):
		return 0, nil
	default:
		return 0, errors.UnknownError.WithFormat("read this node's ledger: %w", err)
	}
}

// peerBlock is the block a peer's state is: what its ledger says.
func (s *PulledState) peerBlock(ctx context.Context, q api.Querier2) (uint64, error) {
	rec, err := q.QueryAccount(ctx, s.partition.JoinPath(protocol.Ledger), nil)
	if err != nil {
		return 0, errors.UnknownError.WithFormat("read a peer's ledger: %w", err)
	}
	ledger, ok := rec.Account.(*protocol.SystemLedger)
	if !ok {
		return 0, errors.Conflict.WithFormat("a peer served %v as %v, not a system ledger",
			s.partition.JoinPath(protocol.Ledger), rec.Account.Type())
	}
	return ledger.Index, nil
}

// staleAccounts is the BPT page diff: the accounts whose leaf this node does
// not hold, or holds and does not agree with. Nothing is written — a leaf
// taken from a peer's word would make the local root the peer's, and the local
// root is what the tracker matches (package enumerate).
func (s *PulledState) staleAccounts(ctx context.Context) ([]*url.URL, error) {
	batch := s.db.Begin(false)
	defer batch.Discard()
	stale, err := enumerate.Stale(ctx, api.Querier2{Querier: s.sources.Querier(s.partition)},
		s.partition, batch, enumerate.Options{})
	if err != nil {
		return stale, errors.UnknownError.WithFormat("find what the peer holds that this node does not: %w", err)
	}
	return stale, nil
}

// heldBatch is one round's fetch: the accounts it pulled and the batch they
// were pulled into. Nothing in it is written until every account in it is
// settled or given up on, because a Pending writes into this batch when it
// settles and a batch cannot be committed while a child of it is open.
type heldBatch struct {
	batch    *database.Batch
	accounts []*heldAccount
	rounds   int
}

// heldAccount is one fetched account waiting for its block to be anchored.
type heldAccount struct {
	url     *url.URL
	pending *pull.Pending
}

// settleHeld settles what earlier rounds fetched and could not verify yet, and
// writes what verified.
//
// This is the hold the Pending/Settle split exists for (package pull). A
// one-shot pull discards on ErrNotAnchored, and the next round re-fetches at a
// newer block the Directory has not anchored either, so nothing ever settles.
// What is held is settled against THE SAME BLOCK, for maxSettleRounds rounds,
// and only then given up and asked for again.
func (s *PulledState) settleHeld(ctx context.Context) (int, []*url.URL) {
	pulled := 0
	var refused []*url.URL
	var keep []*heldBatch

	for _, h := range s.held {
		h.rounds++
		got, missed, done := s.settleBatch(ctx, h)
		pulled += got
		refused = append(refused, missed...)
		if !done {
			keep = append(keep, h)
		}
	}
	s.held = keep
	return pulled, refused
}

// settleBatch settles what it can of one held batch and reports whether the
// batch is finished with.
func (s *PulledState) settleBatch(ctx context.Context, h *heldBatch) (int, []*url.URL, bool) {
	pulled := 0
	var refused []*url.URL
	var waiting []*heldAccount

	for _, a := range h.accounts {
		root, err := s.anchors.AnchoredRoot(ctx, a.pending.Partition, a.pending.Block)
		switch {
		case err == nil:
			if err := a.pending.Settle(root); err != nil {
				s.log.Info("A pulled account did not verify", "account", a.url, "error", err)
				refused = append(refused, a.url)
			} else {
				pulled++
			}
		case errors.Is(err, pull.ErrNotAnchored):
			// The Directory has not anchored that block yet. Held, and asked
			// again next round against the same block.
			waiting = append(waiting, a)
		default:
			a.pending.Discard()
			s.log.Info("The anchored root for a pulled account could not be read",
				"account", a.url, "error", err)
			refused = append(refused, a.url)
		}
	}

	if len(waiting) > 0 && h.rounds < maxSettleRounds && ctx.Err() == nil {
		h.accounts = waiting
		return pulled, refused, false
	}

	// Either everything resolved, or the wait is over. Give up on the
	// stragglers so the batch can close: a block nobody anchors is a peer's
	// claim and not a wait (pull.AccountFrom).
	for _, a := range waiting {
		a.pending.Discard()
		refused = append(refused, a.url)
	}
	h.accounts = nil

	if pulled == 0 {
		h.batch.Discard()
		return 0, refused, true
	}

	// The BPT, then the commit. Batch.Commit commits the BPT store and never
	// calls Account.putBpt, so a perfectly pulled account leaves the local root
	// exactly where it was and the tracker can never match it (#4305).
	if err := h.batch.UpdateBPT(); err != nil {
		s.log.Info("The state tree could not be updated", "partition", s.partition, "error", err)
		h.batch.Discard()
		return 0, refused, true
	}
	if err := h.batch.Commit(); err != nil {
		s.log.Info("What was pulled could not be committed", "partition", s.partition, "error", err)
		return 0, refused, true
	}
	return pulled, refused, true
}

// fetch pulls the accounts, in chunks, and settles what the Directory has
// already anchored. What it has not is held for the next round.
func (s *PulledState) fetch(ctx context.Context, accounts []*url.URL) (int, []*url.URL) {
	pulled := 0
	var refused []*url.URL

	for len(accounts) > 0 && ctx.Err() == nil {
		n := pullChunk
		if n > len(accounts) {
			n = len(accounts)
		}
		chunk := accounts[:n]
		accounts = accounts[n:]

		h := &heldBatch{batch: s.db.Begin(true)}
		for _, u := range chunk {
			srcs, partition, err := s.sourcesFor(ctx, u)
			if err != nil {
				s.log.Info("No peer could be found for an account", "account", u, "error", err)
				refused = append(refused, u)
				continue
			}
			p, _, err := pull.FetchFrom(ctx, srcs, h.batch, u, pull.Options{
				Mode:      pull.ModeStateOnly,
				Verify:    s.anchors,
				Partition: partition,
			})
			if err != nil {
				s.log.Info("An account could not be pulled", "account", u, "error", err)
				refused = append(refused, u)
				continue
			}
			h.accounts = append(h.accounts, &heldAccount{url: u, pending: p})
		}

		if len(h.accounts) == 0 {
			h.batch.Discard()
			continue
		}

		// Settle whatever is already anchored -- a restarting node is often
		// served state at a block the Directory anchored some time ago -- and
		// hold the rest for the next round.
		got, missed, done := s.settleBatch(ctx, h)
		pulled += got
		refused = append(refused, missed...)
		if !done {
			s.held = append(s.held, h)
		}
	}
	return pulled, refused
}

// sourcesFor is the peers that can answer for an account, and the partition
// they answer for. The partition matters: a receipt proves the state as of a
// block, and block numbers collide across partitions (#4308).
func (s *PulledState) sourcesFor(ctx context.Context, u *url.URL) ([]pull.Source, *url.URL, error) {
	srcs, partition, err := s.sources.For(ctx, u)
	if err != nil {
		return nil, nil, errors.UnknownError.Wrap(err)
	}
	if partition == nil {
		partition = s.partition
	}
	return srcs, partition, nil
}

// pullSpine takes THIS PARTITION'S spine — its anchors, ledger and operators,
// with their chains — so anchors and the signatures on them can be read at
// all. It is not verified: it is what the verifier reads from, and there is
// nothing to verify it against until it is there (DIFFERENCES E11). It is
// still taken from a named peer and never from this node.
//
// It does NOT take the Directory's. It used to, and a BVN's state tree holds
// no acc://dn.acme account: writing four of them into a BVN's store puts four
// leaves in its BPT that no peer has, so the local root differs from every
// root the Directory ever anchored for that partition, however perfectly
// everything else is pulled. A node runs the Directory alongside its BVN and
// the Directory's own join pulls the Directory's spine into the Directory's
// store, where those accounts belong.
func (s *PulledState) pullSpine(ctx context.Context) error {
	batch := s.db.Begin(true)
	defer batch.Discard()
	accounts := pull.SpineAccounts(s.partition)
	for _, u := range accounts {
		srcs, partition, err := s.sourcesFor(ctx, u)
		if err != nil {
			return errors.UnknownError.WithFormat("find a peer for the spine account %v: %w", u, err)
		}
		p, _, err := pull.FetchFrom(ctx, srcs, batch, u, pull.Options{
			Mode:      pull.ModeFullSpine,
			Partition: partition,
		})
		if err != nil {
			return errors.UnknownError.WithFormat("pull the spine account %v: %w", u, err)
		}
		if err := p.Keep(); err != nil {
			return errors.UnknownError.WithFormat("keep the spine account %v: %w", u, err)
		}
	}
	err := batch.UpdateBPT()
	if err != nil {
		return errors.UnknownError.WithFormat("update the state tree: %w", err)
	}
	err = batch.Commit()
	if err != nil {
		return errors.UnknownError.WithFormat("commit the spine: %w", err)
	}
	s.log.Info("Pulled the spine", "partition", s.partition, "accounts", len(accounts))
	return nil
}

// Executing records that the node is executing from a block it reached
// without a root match — the whole-network restart, where no peer had staging
// to give and the node starts from its own state. Its services answer for
// themselves again from here; leaving it BOOTING would make a node that is
// running refuse every request for the rest of its life (#4295).
func (s *PulledState) Executing(block uint64) error {
	// The root recorded is this node's own, read BEFORE it starts executing
	// again so that it is the root of the block named. It is not a root
	// anyone anchored — nothing verified this state — and that is the point:
	// this path is taken only when no peer had anything to verify against,
	// because every peer restarted too.
	batch := s.db.Begin(false)
	root, err := batch.GetBptRootHash()
	batch.Discard()
	if err != nil {
		return errors.UnknownError.WithFormat("read this node's own root: %w", err)
	}
	if root == ([32]byte{}) {
		return errors.InternalError.With("this node's root is empty")
	}
	if !s.machine.PromoteToActive(root, block) {
		return errors.Conflict.WithFormat("%v is %v, not joining", s.partition, s.machine.State())
	}
	return nil
}

// Matched reports the block whose anchored root the local root equals. Until
// it does, the node keeps pulling: a root that matches is the only statement
// that the state this node holds is a block's state (executor spec, "Sync").
func (s *PulledState) Matched(ctx context.Context) (uint64, bool, error) {
	ok, err := s.tracker.Check(ctx)
	if err != nil {
		return 0, false, errors.UnknownError.Wrap(err)
	}
	if !ok && s.machine.State() != nodestate.StateActive {
		return 0, false, nil
	}
	return s.machine.Get().SinceBlock, true, nil
}

// dedupe keeps the first of each name and drops the ones no pull can satisfy.
func dedupe(accounts []*url.URL) []*url.URL {
	seen := map[string]bool{}
	out := accounts[:0]
	for _, u := range accounts {
		if !Routable(u) {
			continue
		}
		k := strings.ToLower(u.String())
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, u)
	}
	return out
}
