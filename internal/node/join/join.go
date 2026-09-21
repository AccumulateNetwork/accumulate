// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package join brings a node from "listening" to "executing" (executor spec,
// "Sync"): it collects every block consensus commits, pulls the state a
// verified anchor commits to, and hands off to block production at the first
// block whose transactions it can execute exactly as its peers did.
//
// A restart takes this path too. A node that restarts does not replay the
// blocks it missed and does not rebuild staging from a source's cache: the
// first executes with a staging its peers do not have, the second holds what
// the source produced rather than what the peers had received, and either one
// executes a different block — after which the root chain, a Merkle root over
// the history of block roots, never matches again (#4290, #4205).
//
// # No peer is ever asked what it holds
//
// The earlier design's step 2 — a validator serving its staging as of its
// last committed block — is gone, and so is everything that reached for it:
// `takeStaging`, `LoadStaging`, `ApplyStaging`, `NoPeerHasStaging`, and the
// branch that executed from this node's own state when nobody answered. It
// answered a question the node can answer for itself, and answered it from
// one unauthenticated peer with nothing to check it against (#4322–#4326,
// #4354, #4357). Staging is what this node collected, minus what the pulled
// state says executed, and that is all it ever needs to be.
//
// The two shapes that ended a join wrongly, named so they stay named:
// staging taken from a peer, and executing from an empty stage (#4290).
// Neither is reachable from here any more — there is no call that takes a
// peer's staging, and a node that cannot say what its peers delivered does
// not hand off at all; it keeps collecting, which is the safe state.
package join

import (
	"context"
	"log/slog"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// A Buffer is the consensus side of a join: the node collects every committed
// block into staging and into a buffer, executing none of them, until the
// handoff.
type Buffer interface {
	// Collecting reports whether it is collecting.
	Collecting() bool

	// BufferOverrun reports that more blocks were committed than the buffer
	// holds, so the blocks since the node started listening are no longer all
	// in hand.
	BufferOverrun() bool

	// GapsAt reports whether the block collected as q+1 can be executed: per
	// stream, whether the run from what the PULLED state says was delivered
	// is contiguous through the numbers that block carries.
	//
	// The numbers are that block's, not everything the node has collected. A
	// check against everything collected would call a number that a later
	// block will carry a gap, and the join would never execute anything.
	//
	// errors.NotReady means block q+1 has not been collected yet — a wait,
	// not a gap.
	GapsAt(q uint64) ([]execute.StreamGap, error)

	// Handoff leaves collecting mode at block q and produces the collected
	// block numbered q+1 and every one after it, in order.
	Handoff(q uint64) error
}

// A Stage is the executor's staging half of a join.
type Stage interface {
	// SettleStagingAt brings staging to the block the pulled state is.
	SettleStagingAt(q uint64) error
}

// A State is the state half: it pulls what the node lacks, says which block's
// state the node now holds, and hands what it pulled to the rest of the
// process at the handoff (#4293).
type State interface {
	// Pull runs one round of the pull. It decides for itself what to ask
	// for: the accounts the block ledger says the blocks changed, read from
	// a peer, with the BPT page diff as the backstop (executor spec, "Sync",
	// §3). The caller does not supply a set, because a block's envelopes are
	// not the set (#4306).
	Pull(ctx context.Context) error

	// Matched reports the block whose anchored root the local root equals,
	// and whether it has been reached. It does not promote the node: a node
	// that has the state of block B and cannot yet execute B+1 is still
	// booting.
	Matched(ctx context.Context) (uint64, bool, error)

	// Advance gives up executing from the block the state is at and moves
	// the sync to a later anchored block. It is what a gap is answered with.
	Advance()

	// Handoff publishes what the join pulled — the network definition and
	// the globals — and records that this node is executing from block q.
	// It runs BEFORE the first block is executed.
	Handoff(ctx context.Context, q uint64) error
}

// Options are what a join needs to run.
//
// There is no "fresh" option and there must not be one. Only a node that has
// executed no block may execute without asking anyone (executor spec, "Sync",
// §5), and such a node does not join at all: the daemon does not call Run for
// it (cmd/accumulated/run/dagbft.go, nodeMustJoin). A flag here saying
// "execute anyway" was reachable from no caller for the whole of its life and
// was true in one hand-written test, while the node it described sat in the
// join asking its equally empty peers for staging (#4304).
type Options struct {
	Partition string
	Buffer    Buffer
	Stage     Stage
	State     State
	Logger    *slog.Logger

	// Retry is how long to wait between rounds.
	Retry time.Duration
}

// DefaultRetry is how long a join waits before asking again.
const DefaultRetry = 2 * time.Second

// Run joins the partition. It returns when the node has handed off to block
// production, or when the context is cancelled.
//
// The loop is the spec's (executor spec, "Sync", §4 and §5), and it is four
// sentences:
//
//  1. Sync to B. The pull asks for the state as of a block a quorum signed an
//     anchor for, holds that block fixed for the whole pass, and the pass is
//     over when the local root equals that block's anchored root.
//  2. Collect B+1. Every block consensus commits has been going into staging
//     and into the buffer since the node started listening, so B+1 is there
//     or it is a moment away.
//  3. If no stream has a gap — no sequence number missing between what the
//     pulled state says was delivered and what B+1 carries — execute B+1.
//     That is the handoff, and the join is done.
//  4. A gap is an entry that arrived before this node was listening, and
//     executing without it is the #4290 divergence. Do not execute. Keep
//     B+1 staged, advance the sync to the next anchored block — everything
//     the node lacks below it is then at or under the new Delivered and is
//     no longer a gap — and ask the same question of the block after that.
//
// It terminates because everything that arrives after the node starts
// listening it holds, so a gap can only be an entry from before, and held
// sets are small and clear within a few blocks.
func Run(ctx context.Context, opts Options) error {
	if opts.Buffer == nil || opts.Stage == nil || opts.State == nil {
		return errors.BadRequest.With("a join needs a buffer, a stage and a state")
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

	for {
		if err := ctx.Err(); err != nil {
			return errors.UnknownError.Wrap(err)
		}
		if opts.Buffer.BufferOverrun() {
			// The blocks since the node started listening are no longer all
			// in hand, so it cannot say which collected group is which block
			// and it must not guess. There is nothing to start again from —
			// the buffer is the only copy — so this is a fault the node is
			// restarted out of, and it keeps collecting nothing meanwhile.
			return errors.FatalError.WithFormat(
				"the join buffer overran: the blocks this node collected are no longer all in hand, so it cannot say which of them is the block after the state it pulled")
		}

		b, synced, err := opts.State.Matched(ctx)
		if err != nil {
			return errors.UnknownError.WithFormat("match the anchored root: %w", err)
		}

		if synced {
			done, err := converge(ctx, opts, log, b)
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
			if done {
				return nil
			}
		} else {
			err = opts.State.Pull(ctx)
			if err != nil {
				return errors.UnknownError.WithFormat("pull state: %w", err)
			}
		}

		select {
		case <-ctx.Done():
			return errors.UnknownError.Wrap(ctx.Err())
		case <-time.After(retry):
		}
	}
}

// converge is steps 2 to 4 for one synced block: it asks whether the block
// collected as b+1 can be executed, and either hands off or advances.
func converge(ctx context.Context, opts Options, log *slog.Logger, b uint64) (bool, error) {
	gaps, err := opts.Buffer.GapsAt(b)
	switch {
	case errors.Is(err, errors.NotReady):
		// The state ran ahead of the blocks consensus has delivered. Waiting
		// is the whole of the answer: the block is on its way.
		log.Info("The state is block B and the block after it has not been collected yet; waiting",
			"block", b, "reason", err)
		return false, nil

	case err != nil:
		return false, errors.UnknownError.WithFormat("check the collected block %d for gaps: %w", b+1, err)

	case len(gaps) == 0:
		// Nothing any peer is holding is missing here. The node executes
		// b+1 as its peers did.
		//
		// The order is not interchangeable. The globals the join pulled are
		// published first, because the membership gate, the anchor gate and
		// the adapter's committee all read that one event and the first
		// block must not run on the definition this node had before it
		// joined. Then staging is settled at b, which is what makes the
		// stage say what its peers' stage says at b. Only then are blocks
		// produced.
		err = opts.State.Handoff(ctx, b)
		if err != nil {
			return false, errors.UnknownError.WithFormat("hand off the pulled state at %d: %w", b, err)
		}
		err = opts.Stage.SettleStagingAt(b)
		if err != nil {
			return false, errors.UnknownError.WithFormat("settle staging at %d: %w", b, err)
		}
		err = opts.Buffer.Handoff(b)
		if err != nil {
			return false, errors.UnknownError.WithFormat("hand off at %d: %w", b, err)
		}
		log.Info("Joined: the state is block B and the block after it has no gap", "block", b)
		return true, nil
	}

	// A gap. It is an entry that arrived before this node was listening —
	// the peers held it from a block before b — and executing b+1 without it
	// is the divergence the join exists to prevent (#4290). Said out loud,
	// because a join that advances for ever and a join that is stuck look
	// identical in a log that does not name the stream.
	for _, g := range gaps {
		log.Info("The block after the state has a gap: an entry that arrived before this node was listening",
			"block", b+1, "stream", g.ID.Ledger, "source", g.ID.Source,
			"delivered", g.Delivered, "through", g.Through, "missing", g.Missing)
	}
	opts.State.Advance()
	return false, nil
}
