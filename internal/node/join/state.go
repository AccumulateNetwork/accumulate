// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"log/slog"

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

// PulledState is the join's state half (#4293): it pulls what the node lacks
// from running peers, verified against the root the Directory anchored for
// the block the peer served it at, and says when the local root equals one of
// those roots — the block the node then executes from (executor spec, "Sync",
// steps 3 and 4).
//
// What it pulls is what the blocks name: the accounts every collected block's
// envelopes touched (#4292 returns them). A round in which the blocks named
// nothing falls back to the BPT page diff, which is slower and finds what a
// quiet block does not mention.
type PulledState struct {
	partition *url.URL
	db        *database.Database
	source    pull.Source
	query     api.Querier
	anchors   *pull.DirectoryAnchors
	machine   *nodestate.Machine
	tracker   *tracker.Tracker
	log       *slog.Logger

	spine bool // the Directory's spine has been pulled

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

	// Query reaches the network, routed: the Directory for its anchors, and
	// whichever partition an account belongs to for its state.
	Query api.Querier

	Logger *slog.Logger
}

// NewState wires the pull, the anchor reader and the tracker together.
func NewState(opts StateOptions) (*PulledState, error) {
	if opts.Partition == nil || opts.Database == nil || opts.Query == nil {
		return nil, errors.BadRequest.With("a join's state needs a partition, a database and a querier")
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
		source:    api.Querier2{Querier: opts.Query},
		query:     opts.Query,
		machine:   machine,
		tracker:   track,
		log:       log.With("module", "join"),
	}
	s.anchors = &pull.DirectoryAnchors{
		Query:    opts.Query,
		OnAnchor: track.Observe,
	}

	// The partition's services read this to decide what they may answer: a
	// node that is joining answers for nothing it has not executed (#4295).
	// Registered as BOOTING, and the tracker promotes it to ACTIVE when the
	// root matches.
	if id, ok := protocol.ParsePartitionUrl(opts.Partition); ok {
		nodestate.Register(id, machine)
	}
	return s, nil
}

// Machine is the node's state — BOOTING until the root matches, ACTIVE after
// — which the partition's services refuse requests by (#4295).
func (s *PulledState) Machine() *nodestate.Machine { return s.machine }

// Pull fetches the accounts the collected blocks named, and the Directory's
// spine the first time. Every account is verified against the root the
// Directory anchored for the block it was served at; one that cannot be
// verified is not written, and is asked for again next round.
func (s *PulledState) Pull(ctx context.Context, accounts []*url.URL) error {
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

	// What could not be pulled last round is asked for again, with whatever
	// this round's blocks named.
	if len(s.refused) > 0 {
		accounts = append(append([]*url.URL{}, s.refused...), accounts...)
		s.refused = nil
	}

	if len(accounts) == 0 {
		// A round the blocks named nothing for: find what differs by paging
		// the peer's BPT against this node's leaves. Incomplete on a live
		// tree by construction (DIFFERENCES E11), which is why the blocks
		// are the primary source.
		batch := s.db.Begin(false)
		stale, err := enumerate.Stale(ctx, api.Querier2{Querier: s.query}, s.partition, batch, enumerate.Options{})
		batch.Discard()
		if err != nil {
			return errors.UnknownError.WithFormat("find what the peer holds that this node does not: %w", err)
		}
		accounts = stale
	}
	if len(accounts) == 0 {
		return nil
	}

	batch := s.db.Begin(true)
	defer batch.Discard()
	pulled := 0
	var refused []*url.URL
	seen := map[string]bool{}
	for _, u := range accounts {
		if seen[u.String()] {
			continue
		}
		seen[u.String()] = true
		err := pull.Account(ctx, s.source, batch, u, pull.Options{
			Mode:      pull.ModeStateOnly,
			Verify:    s.anchors,
			Partition: s.partition,
		})
		switch {
		case err == nil:
			pulled++
		case errors.Is(err, pull.ErrNotAnchored):
			// Served at a block the Directory has not anchored yet: asked
			// for again next round, when it has.
			refused = append(refused, u)
		default:
			s.log.Info("An account could not be pulled", "account", u, "error", err)
			refused = append(refused, u)
		}
	}
	s.refused = refused
	err = batch.Commit()
	if err != nil {
		return errors.UnknownError.WithFormat("commit what was pulled: %w", err)
	}
	s.log.Info("Pulled the accounts the blocks named", "partition", s.partition,
		"asked", len(accounts), "pulled", pulled, "refused", len(refused))
	return nil
}

// pullSpine takes the Directory's spine — its anchors, ledger and operators,
// with their chains — and this partition's own, so anchors and the signatures
// on them can be read at all. It is not verified: it is what the verifier
// reads from, and there is nothing to verify it against until it is there
// (DIFFERENCES E11).
func (s *PulledState) pullSpine(ctx context.Context) error {
	batch := s.db.Begin(true)
	defer batch.Discard()
	accounts := pull.DnSpineAccounts()
	if !protocol.DnUrl().Equal(s.partition) {
		accounts = append(accounts, pull.SpineAccounts(s.partition)...)
	}
	for _, u := range accounts {
		err := pull.Account(ctx, s.source, batch, u, pull.Options{Mode: pull.ModeFullSpine})
		if err != nil {
			return errors.UnknownError.WithFormat("pull the spine account %v: %w", u, err)
		}
	}
	err := batch.Commit()
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
func (s *PulledState) Executing(block uint64) {
	// The anchor recorded is this node's own root at that block. It is not a
	// root anyone anchored — nothing verified this state — and the difference
	// is the point: this path is taken only when no peer had anything to
	// verify against, because every peer restarted too.
	batch := s.db.Begin(false)
	defer batch.Discard()
	root, err := batch.GetBptRootHash()
	if err != nil {
		s.log.Error("Cannot read this node's own root", "error", err)
		return
	}
	s.machine.PromoteToActive(root, block)
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
