// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package join brings a node from "listening" to "executing" (executor spec,
// "Sync"): it collects every block consensus commits into its own staging,
// pulls the state a signed anchor proves, and hands off to block production
// at the first block whose streams have no gap below what it carries.
//
// A restart takes this path too. A node that restarts does not replay the
// blocks it missed and does not rebuild staging from a source's cache: the
// first executes with a staging its peers do not have, the second holds what
// the source produced rather than what the peers had received, and either one
// executes a different block — after which the root chain, a Merkle root over
// the history of block roots, never matches again (#4290, #4205).
package join

import (
	"context"
	"log/slog"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// A Buffer is the consensus side of a join: the node collects every committed
// block into staging and keeps it, executing none of them, until the handoff
// (#4292's collecting mode).
type Buffer interface {
	// StartCollecting puts the node in collecting mode.
	StartCollecting()

	// Collecting reports whether it is collecting.
	Collecting() bool

	// BufferOverrun reports that more blocks were committed than the buffer
	// holds, so the blocks since the node started listening are no longer
	// all in hand.
	BufferOverrun() bool

	// ApplyStaging runs load, then applies every block buffered since this
	// node started collecting to staging, in order, and each new one as it
	// arrives. The join loads nothing: staging is what the node collected
	// from consensus, minus what the pulled state says executed (executor
	// spec, "Sync", step 4), so the join passes a load that does nothing.
	ApplyStaging(load func() error) error

	// Handoff leaves collecting mode at block q and produces the buffered
	// groups from q + 1 in order.
	Handoff(q uint64) error
}

// A Stage is the executor's staging half of a join (#4292).
type Stage interface {
	// LoadStaging takes a peer's staging into this node's own. Run does not
	// call it (#4362).
	LoadStaging(*private.StagingSnapshot) error

	// SettleStagingAt brings staging to the block the pulled state is.
	SettleStagingAt(q uint64) error

	// HasGap reports whether a stream cannot run contiguously from the
	// Delivered the pulled state holds when block is executed next: a
	// number between Delivered and what staging holds that nothing is held
	// for. Such a number is an entry that arrived before the node was
	// listening, and executing block without it is the #4290 divergence
	// (executor spec, "Sync", step 4).
	HasGap(block uint64) (bool, error)
}

// A State is the state half: it pulls what the node lacks and says when the
// local root equals a root the Directory anchored (#4293).
type State interface {
	// Pull fetches what the node lacks, verified against the anchored root.
	// It decides for itself what that is: the accounts the block ledger says
	// the blocks changed, read from a peer, with the BPT page diff as the
	// backstop (executor spec, "Sync", step 3). The caller does not supply a
	// set, because a block's envelopes are not the set (#4306).
	Pull(ctx context.Context) error

	// Matched reports the block whose anchored root the local root equals,
	// and whether it has been reached. It follows the state: after a pull
	// that advanced the sync, it reports the block the sync advanced to.
	Matched(ctx context.Context) (uint64, bool, error)
}

// A Peers finds the partition's other validators and their private API.
type Peers interface {
	// Validators lists the nodes serving this partition's sequencer, in no
	// particular order.
	Validators(ctx context.Context) ([]*api.FindServiceResult, error)

	// Staging is the private API addressed to one node. Run does not call
	// it (#4362).
	Staging(peer *api.FindServiceResult) private.StagingSnapshotter
}

// Options are what a join needs to run.
//
// There is no "fresh" option and there must not be one. Only a node that has
// executed no block may execute without asking anyone (executor spec, "Sync",
// step 5), and such a node does not join at all: the daemon does not call Run
// for it (cmd/accumulated/run/dagbft.go, nodeMustJoin). A flag here saying
// "execute anyway" was reachable from no caller for the whole of its life and
// was true in one hand-written test (#4304). Everything that reaches Run has
// executed a block, so finding no validator is never an answer for it.
type Options struct {
	Partition string
	Buffer    Buffer
	Stage     Stage
	State     State
	Peers     Peers
	Logger    *slog.Logger

	// Retry is how long to wait before asking again when no validator can
	// be found, or when the sync has not reached a block it can execute.
	Retry time.Duration

	// Rounds is how many times the partition's validators are looked for
	// before the join gives up on finding any. Zero means DefaultRounds.
	Rounds int
}

// DefaultRetry is how long a join waits before asking again.
const DefaultRetry = 2 * time.Second

// DefaultRounds is how many times a join looks for the partition's
// validators before it concludes that it cannot see its partition.
const DefaultRounds = 10

// An Outcome is how a join ended.
type Outcome int

const (
	// Joined: the node synced to a block whose successor has no gap,
	// settled staging there and handed off. It is executing.
	Joined Outcome = iota
)

// Run joins the partition. It returns when the node has handed off to block
// production, or when the context is cancelled.
//
// It converges block by block (executor spec, "Sync", step 4):
//
//  1. listen — collecting mode, so no committed block is executed, every one
//     is kept, and every one is taken into this node's own staging;
//  2. sync to a block B: pull until the local root equals a proven root;
//  3. ask whether B + 1 has a gap. If not, settle staging at B and hand off:
//     B + 1 executes from the buffer as any node executes a block. If it
//     has, B + 1 is not executed; the pull advances the sync to the state
//     that names it, whose Delivered says what the peers actually ran, and
//     the question is asked of the block after that.
//
// It ends because everything that arrived after the node started listening,
// the node holds; a gap is only an entry from before, and the network
// executes those within a few blocks. No peer is asked what it holds.
func Run(ctx context.Context, opts Options) (Outcome, error) {
	if opts.Buffer == nil || opts.Stage == nil || opts.State == nil || opts.Peers == nil {
		return Joined, errors.BadRequest.With("a join needs a buffer, a stage, a state and peers")
	}
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	log = log.With("module", "join", "partition", opts.Partition)
	retry := opts.Retry
	if retry <= 0 {
		retry = DefaultRetry
	}

	// Step 1. Collecting starts before anything is pulled, so every block
	// committed after the state the pull reaches is one this node has
	// collected. Staging is this node's own from the first block: nothing is
	// loaded into it.
	opts.Buffer.StartCollecting()
	err := opts.Buffer.ApplyStaging(func() error { return nil })
	if err != nil {
		return Joined, errors.UnknownError.WithFormat("collect into staging: %w", err)
	}

	// A node that cannot see its partition cannot know what its peers have
	// executed, and executing from its own state is exactly the divergence
	// the join exists to prevent (#4290, #4296). It keeps collecting, which
	// is the safe state, and says so. There is no exception: a node that has
	// executed nothing never gets here (#4304).
	found, err := findValidators(ctx, opts, log, retry)
	if err != nil {
		return Joined, errors.UnknownError.Wrap(err)
	}
	if found == 0 {
		return Joined, errors.NotReady.WithFormat(
			"no validator of %s could be found; this node is collecting and not executing", opts.Partition)
	}

	for {
		if err := ctx.Err(); err != nil {
			return Joined, errors.UnknownError.Wrap(err)
		}
		if opts.Buffer.BufferOverrun() {
			// A committed block is missing from the buffer, so the blocks
			// after the state are no longer all in hand and none of them may
			// be produced. Starting to collect again would lose the mapping
			// from buffered groups to blocks (#4351), so the node stays
			// collecting and says so.
			return Joined, errors.NotReady.With("the join buffer overran; this node is collecting and not executing")
		}

		q, ok, err := opts.State.Matched(ctx)
		if err != nil {
			return Joined, errors.UnknownError.WithFormat("match the anchored root: %w", err)
		}
		if ok {
			gap, err := opts.Stage.HasGap(q + 1)
			if err != nil {
				return Joined, errors.UnknownError.WithFormat("look for a gap at %d: %w", q+1, err)
			}
			if gap {
				// An entry from before the node was listening: the peers
				// hold it and this node does not. The sync advances instead.
				log.Info("The next block has a gap; advancing the sync", "synced", q, "block", q+1)
			} else {
				err = opts.Stage.SettleStagingAt(q)
				if err != nil {
					return Joined, errors.UnknownError.WithFormat("settle staging at %d: %w", q, err)
				}
				err = opts.Buffer.Handoff(q)
				switch {
				case err == nil:
					log.Info("Joined; executing from the block after the state", "block", q, "executes", q+1)
					return Joined, nil
				case errors.Is(err, errors.NotReady):
					// The pull ran ahead of the blocks consensus has
					// delivered: handing off now would give the blocks still
					// to arrive the wrong numbers. Wait for them.
					log.Info("The state is ahead of the blocks collected so far; waiting", "block", q, "error", err)
				default:
					return Joined, errors.UnknownError.WithFormat("hand off at %d: %w", q, err)
				}
			}
		}

		err = opts.State.Pull(ctx)
		if err != nil {
			return Joined, errors.UnknownError.WithFormat("pull state: %w", err)
		}

		select {
		case <-ctx.Done():
			return Joined, errors.UnknownError.Wrap(ctx.Err())
		case <-time.After(retry):
		}
	}
}

// findValidators looks for the partition's validators, up to Rounds times,
// and returns how many it found. Zero means the node could not see its
// partition at all.
func findValidators(ctx context.Context, opts Options, log *slog.Logger, retry time.Duration) (int, error) {
	rounds := opts.Rounds
	if rounds <= 0 {
		rounds = DefaultRounds
	}
	for attempt := 0; attempt < rounds; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return 0, errors.UnknownError.Wrap(ctx.Err())
			case <-time.After(retry):
			}
		}
		peers, err := opts.Peers.Validators(ctx)
		if err != nil {
			log.Info("Cannot find this partition's validators yet", "error", err)
			continue
		}
		if len(peers) > 0 {
			return len(peers), nil
		}
		log.Info("No validator of this partition has been found yet", "attempt", attempt+1, "of", rounds)
	}
	return 0, nil
}

// takeStaging asks the partition's validators, one by one, for their staging,
// and loads the first answer that is usable. A validator that refuses, that
// cannot be read, or that has executed no block itself is passed over: that
// is that node's condition, not an answer about the snapshot, and the next
// validator is asked.
//
// Run no longer calls it: the join takes no peer's staging (#4362).
func takeStaging(ctx context.Context, opts Options, log *slog.Logger, retry time.Duration) (uint64, bool, int, error) {
	rounds := opts.Rounds
	if rounds <= 0 {
		rounds = DefaultRounds
	}
	// How many distinct validators were found and asked across every round.
	// Zero means the node could not see its partition at all, which is a
	// different thing from every validator having nothing to give (#4296).
	asked := map[string]bool{}
	for attempt := 0; ; attempt++ {
		if attempt >= rounds {
			return 0, false, len(asked), nil
		}
		if err := ctx.Err(); err != nil {
			return 0, false, len(asked), errors.UnknownError.Wrap(err)
		}
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return 0, false, len(asked), errors.UnknownError.Wrap(ctx.Err())
			case <-time.After(retry):
			}
		}

		peers, err := opts.Peers.Validators(ctx)
		if err != nil {
			log.Info("Cannot find this partition's validators yet", "error", err)
			continue
		}
		if len(peers) == 0 {
			log.Info("No validator of this partition has been found yet", "attempt", attempt+1, "of", rounds)
		}
		for _, peer := range peers {
			asked[peer.PeerID.String()] = true
			snap, err := private.FetchStagingSnapshot(ctx, opts.Peers.Staging(peer), opts.Partition)
			if err != nil {
				log.Info("A validator did not serve its staging", "peer", peer.PeerID, "error", err)
				continue
			}
			if snap.Block == 0 {
				continue // that node has executed no block: it is joining too
			}

			// The buffer holds every block after P, and this is why: the node
			// entered collecting mode before it asked, so every block the
			// peer committed after it answered — every block above P — is a
			// block this node has collected. Everything at or below P is in
			// the snapshot. There is no third case, and no block index to
			// compare: a buffered group carries a leader round, not a block.
			err = opts.Buffer.ApplyStaging(func() error { return opts.Stage.LoadStaging(snap) })
			if err != nil {
				log.Info("A validator's staging could not be loaded", "peer", peer.PeerID, "block", snap.Block, "error", err)
				continue
			}
			return snap.Block, true, len(asked), nil
		}
	}
}
