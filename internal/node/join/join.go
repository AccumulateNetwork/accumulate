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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// mHandoffFailures counts the handoffs that could not be performed. A join
// retries a failed handoff without bound (executor spec, "Sync", step 5;
// #4401), so a failure that recurs on every attempt shows as this count
// climbing, not as silence.
var mHandoffFailures = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "accumulate",
	Subsystem: "join",
	Name:      "handoff_failures_total",
	Help:      "Handoffs that failed and sent the join back to syncing",
}, []string{"partition"})

// mGapMissing is, per stream, the first number the last gap check found
// nothing held for. A check replaces the partition's series, so a stream is
// on the gauge only while its gap stands, and a join that found none has no
// series (#4432).
var mGapMissing = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "accumulate",
	Subsystem: "join",
	Name:      "gap_first_missing",
	Help: "The first sequence number above Delivered that the join's last gap check found " +
		"nothing held for, per stream (source->ledger); absent when the stream has no gap",
}, []string{"partition", "stream"})

// reportGaps puts the gap check's answer on the gauge.
func reportGaps(partition string, gaps []StreamGap) {
	mGapMissing.DeletePartialMatch(prometheus.Labels{"partition": partition})
	for _, g := range gaps {
		mGapMissing.WithLabelValues(partition, g.StreamName()).Set(float64(g.Missing))
	}
}

// A Buffer is the consensus side of a join: the node keeps every committed
// block, executing none of them, until the handoff (#4292's collecting mode).
// It takes them into staging only through the block after the state the join
// proves (#4398).
type Buffer interface {
	// StartCollecting puts the node in collecting mode. While collecting it
	// keeps the buffer (#4351), unless the buffer has overrun: then it starts
	// a new one, from the block the node stands at now.
	StartCollecting()

	// Collecting reports whether it is collecting.
	Collecting() bool

	// BufferOverrun reports that more blocks were committed than the buffer
	// holds, so the blocks since the node started listening are no longer
	// all in hand.
	BufferOverrun() bool

	// StageThrough takes into staging, in order, every buffered block
	// through block and none after it (executor spec, "Sync", step 5;
	// #4398). The join calls it with Q + 1 once the state matches at Q:
	// staging is what the node collected from consensus through Q + 1, minus
	// what the pulled state says executed, and the blocks after Q + 1 reach
	// staging only by being executed. Staging only grows: a later call takes
	// in the blocks between. NotReady: block has not been collected yet.
	// Conflict: the state is not one this node can hand off from.
	StageThrough(block uint64) error

	// Handoff leaves collecting mode at block q and produces, as q + 1,
	// q + 2, …, the buffered groups committed at a leader round above the
	// one q's system ledger records (executor spec, "Sync", step 5; #4362).
	// NotReady: the groups up to that round have not all arrived, or the
	// state records no round. Conflict: the state is behind what this node
	// can produce from, and the join pulls again. Any other error is a
	// group that could not be produced: the buffer is collecting again,
	// holding the groups it did not produce, and the join syncs again
	// (#4401).
	Handoff(q uint64) error
}

// A Stage is the executor's staging half of a join (#4292).
type Stage interface {
	// SettleStagingAt brings staging to the block the pulled state is.
	SettleStagingAt(q uint64) error

	// HasGap reports whether a stream cannot run contiguously from the
	// Delivered the pulled state holds when block is executed next: a
	// number between Delivered and what staging holds that nothing is held
	// for. Such a number is an entry that arrived before the node was
	// listening, and executing block without it is the #4290 divergence
	// (executor spec, "Sync", step 4).
	// The answer is every stream with such a number, empty when there is
	// none: the join logs it and the gauge carries it (#4432).
	HasGap(block uint64) ([]StreamGap, error)
}

// A State is the state half: it pulls what the node lacks and says when the
// local root equals a root the partition's validators signed (#4293).
type State interface {
	// Pull runs one round of the pull: the block-ledger records since the
	// last one processed, then the next pages of the walk of the peer's BPT
	// (executor spec, "Sync", "The algorithm", steps 1-2). It decides for
	// itself what to pull; the caller does not supply a set, because a
	// block's envelopes are not the set (#4306).
	Pull(ctx context.Context) error

	// Matched reports the block whose anchored root the local root equals,
	// and whether it has been reached. It follows the state: after a pull
	// that advanced the sync, it reports the block the sync advanced to.
	Matched(ctx context.Context) (uint64, bool, error)

	// Promote says the node handed off at block, the block Matched last
	// reported, and is executing from the block after it: it is ACTIVE from
	// here. A match alone is not ACTIVE — a node that matched and has not
	// handed off executes nothing (executor spec, "Sync", step 6; #4385).
	Promote(block uint64)

	// Demote says the node stopped executing in agreement at block: its root
	// diverged and it is syncing again, or it matched there and its handoff
	// failed. The node is BOOTING from here — it refuses every read, serves
	// nothing and relays every submission — until a handoff succeeds again
	// and Promote is called (executor spec, "Sync", steps 4-6; #4385).
	//
	// Both are part of State, not optional extras, so that a State that
	// wraps another cannot drop them without failing to compile.
	Demote(block uint64)
}

// A Peers finds the partition's other validators and their private API.
type Peers interface {
	// Validators lists the nodes serving this partition's sequencer, in no
	// particular order.
	Validators(ctx context.Context) ([]*api.FindServiceResult, error)
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
// production and its state cannot check the blocks it executes, or when the
// context is cancelled.
//
// It converges block by block (executor spec, "Sync", step 4):
//
//  1. listen — collecting mode, so no committed block is executed and every
//     one is kept;
//  2. sync to a block B: pull until the local root equals a proven root;
//  3. take the kept blocks through B + 1 into this node's own staging, and
//     none after it; ask whether B + 1 has a gap. If not, settle staging at B
//     and hand off:
//     B + 1 executes from the buffer as any node executes a block. If it
//     has, B + 1 is not executed; the pull advances the sync to the state
//     that names it, whose Delivered says what the peers actually ran, and
//     the question is asked of the block after that;
//  4. after the handoff, if the state is a RootWatch, check every executed
//     block's root against the root the Directory anchored for it. A
//     mismatch is a gap the sequence check missed: the node collects again,
//     pulls, and goes back to step 3 from the state it reaches, so a wrong
//     run is caught at the block it happens in and never carried forward.
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
	// collected. Staging is this node's own: nothing is loaded into it.
	opts.Buffer.StartCollecting()

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

	q, err := converge(ctx, opts, log, retry, false)
	if err != nil {
		return Joined, errors.UnknownError.Wrap(err)
	}

	// A state that can check the blocks this node executes keeps checking
	// them for as long as the node runs; one that cannot has joined.
	watch, ok := opts.State.(RootWatch)
	if !ok {
		return Joined, nil
	}
	for {
		if h, ok := opts.State.(interface{ HandedOff(uint64) }); ok {
			h.HandedOff(q)
		}
		n, err := watchRoots(ctx, watch, log, retry)
		if err != nil {
			return Joined, errors.UnknownError.Wrap(err)
		}
		if ctx.Err() != nil {
			// The node is executing and is being stopped: the join ended
			// as it should, it did not fail.
			return Joined, nil
		}

		// Block n's root is not the root the Directory anchored for it.
		// The node stops executing and syncs again from where it is, as
		// it did the first time; the blocks from here are collected and
		// none of them is executed until the root matches again. It stops
		// answering first: its state is known wrong, and an ACTIVE node
		// serving it is what run 20260924T074702Z measured (#4385).
		log.Warn("An executed block's root differs from its proven root; syncing again", "block", n)
		opts.State.Demote(n)
		opts.Buffer.StartCollecting()

		// The pull comes first: the local root still equals the block the
		// node handed off at, and matching it again would hand off where
		// the divergence began.
		q, err = converge(ctx, opts, log, retry, true)
		if err != nil {
			return Joined, errors.UnknownError.Wrap(err)
		}
	}
}

// A RootWatch is a State that can say whether a block this node executed
// after the handoff has a root other than the one the Directory anchored for
// it. After executing any block the local root equals that block's proven
// root or it does not, and a mismatch is a gap the sequence check missed
// (executor spec, "Sync", step 4). A proof arrives blocks after the block it
// proves, so "not proven yet" is not a mismatch.
type RootWatch interface {
	// Diverged reports the first executed block whose local root differs
	// from its proven root, and whether there is one.
	Diverged(ctx context.Context) (uint64, bool, error)
}

// watchRoots asks the watch every retry until a block diverges or the
// context ends. It returns the block that diverged.
func watchRoots(ctx context.Context, watch RootWatch, log *slog.Logger, retry time.Duration) (uint64, error) {
	for {
		select {
		case <-ctx.Done():
			return 0, nil
		case <-time.After(retry):
		}

		n, diverged, err := watch.Diverged(ctx)
		switch {
		case ctx.Err() != nil:
			return 0, nil
		case err != nil:
			// A peer that cannot serve an anchor this second says nothing
			// about this node's roots. The next round asks again.
			log.Info("The executed blocks' roots could not be checked this round", "error", err)
		case diverged:
			return n, nil
		}
	}
}

// converge syncs to a block whose successor has no gap, settles staging there
// and hands off, and returns that block (steps 2 and 3 of Run). pullFirst
// pulls before the first match: a node syncing again holds a state that
// matched once and no longer does.
func converge(ctx context.Context, opts Options, log *slog.Logger, retry time.Duration, pullFirst bool) (uint64, error) {
	if pullFirst {
		err := opts.State.Pull(ctx)
		if err != nil {
			return 0, errors.UnknownError.WithFormat("pull state: %w", err)
		}
	}
	overran := false
	failures := 0
	for {
		if err := ctx.Err(); err != nil {
			return 0, errors.UnknownError.Wrap(err)
		}
		if opts.Buffer.BufferOverrun() {
			// A committed block is missing from the buffer, so the blocks
			// after the state are no longer all in hand and none of them may
			// be produced. The join does not end here: the daemon runs it
			// once, so ending it would leave the node collecting until a
			// restart. It starts collecting again, from the network's block
			// now, and syncs to a block past it. A buffer that has not
			// overrun keeps what it holds when asked to collect (#4351).
			if !overran {
				log.Warn("The join buffer overran; collecting again and syncing to a newer block")
				overran = true
			}
			opts.Buffer.StartCollecting()
			if opts.Buffer.BufferOverrun() {
				// The buffer did not start again. Nothing may be handed
				// off from it, so the node stays collecting and asks again.
				select {
				case <-ctx.Done():
					return 0, errors.UnknownError.Wrap(ctx.Err())
				case <-time.After(retry):
				}
				continue
			}
			log.Info("Collecting again after the join buffer overran")
			overran = false
		}

		q, proven, err := opts.State.Matched(ctx)
		if err != nil {
			return 0, errors.UnknownError.WithFormat("match the anchored root: %w", err)
		}
		// The node executes only from a state that matched (executor spec,
		// "Sync", "Two mismatches", 1).
		if proven {
			done, err := stageAndHandOff(opts, log, q, proven, &failures)
			if err != nil {
				return 0, errors.UnknownError.Wrap(err)
			}
			if done {
				return q, nil
			}
		}

		err = opts.State.Pull(ctx)
		if err != nil {
			return 0, errors.UnknownError.WithFormat("pull state: %w", err)
		}

		select {
		case <-ctx.Done():
			return 0, errors.UnknownError.Wrap(ctx.Err())
		case <-time.After(retry):
		}
	}
}

// stageAndHandOff stages the kept blocks through q + 1, asks whether q + 1 has
// a gap, and if not settles staging at q and hands off. proven says the state
// at q matched a signed anchor; only then is the node promoted here, and
// otherwise at the first executed block that matches. It reports whether the
// node handed off; false with no error means the join pulls again. failures
// counts the handoffs that failed.
func stageAndHandOff(opts Options, log *slog.Logger, q uint64, proven bool, failures *int) (bool, error) {
	// Staging holds everything collected through q + 1 and nothing after it
	// (#4398): the gap check asks what q + 1 carries, and a block delivers
	// the run it can from what is held, so a staging that also held what
	// q + 2 and later brought would execute q + 1 differently than the peers
	// did.
	err := opts.Buffer.StageThrough(q + 1)
	switch {
	case err == nil:
	case errors.Is(err, errors.NotReady):
		log.Info("The block after the state has not been collected yet; waiting", "synced", q, "block", q+1, "error", err)
		return false, nil
	case errors.Is(err, errors.Conflict):
		log.Info("The state is not one this node can hand off from; pulling again", "block", q, "error", err)
		return false, nil
	default:
		return false, errors.UnknownError.WithFormat("stage through %d: %w", q+1, err)
	}

	gaps, err := opts.Stage.HasGap(q + 1)
	if err != nil {
		return false, errors.UnknownError.WithFormat("look for a gap at %d: %w", q+1, err)
	}
	reportGaps(opts.Partition, gaps)
	if len(gaps) > 0 {
		// An entry from before the node was listening: the peers hold it and
		// this node does not. The node executes anyway, with whatever
		// staging holds: a block executed without an entry it needed is
		// wrong only in the accounts its record names, and the root check
		// repairs them from the block ledger (executor spec, "Sync", "Two
		// mismatches", and step 6). The gap is said and put on the gauge.
		streams := make([]string, len(gaps))
		for i, g := range gaps {
			streams[i] = g.String()
		}
		log.Info("The next block has a gap; executing anyway, and repairing on a mismatch", "synced", q, "block", q+1, "gaps", streams)
	}

	err = opts.Stage.SettleStagingAt(q)
	if err != nil {
		return false, errors.UnknownError.WithFormat("settle staging at %d: %w", q, err)
	}
	err = opts.Buffer.Handoff(q)
	switch {
	case err == nil:
		// The node is executing from q + 1, in agreement: this, and not the
		// match, is where it becomes ACTIVE (#4385). A handoff that is
		// refused or fails below never promotes, so a retried handoff never
		// flips the node's state.
		if !proven {
			log.Info("Executing from the pulled state, unproven; comparing at every block that anchors", "block", q, "executes", q+1)
			return true, nil
		}
		log.Info("Joined; executing from the block after the state", "block", q, "executes", q+1)
		opts.State.Promote(q)
		return true, nil
	case errors.Is(err, errors.NotReady):
		// The pull ran ahead of the blocks consensus has delivered: handing
		// off now would give the blocks still to arrive the wrong numbers.
		// Wait for them.
		log.Info("The state is ahead of the blocks collected so far; waiting", "block", q, "error", err)
		return false, nil
	case errors.Is(err, errors.Conflict):
		// Syncing again, the state matched a block before the one this node
		// had executed to. Those blocks are not in the buffer, so the node
		// cannot execute from there; the pull goes on to the peers' newer
		// state.
		log.Info("The state is behind the block this node stood at; pulling again", "block", q, "error", err)
		return false, nil
	default:
		// A group could not be produced. The join does not end: the node
		// syncs again and hands off again, counted, because a node that
		// stops here neither executes nor collects (executor spec, "Sync",
		// step 5; #4401). A buffer that went back to collecting keeps the
		// groups it did not produce; one that did not starts again.
		// The node matched at q and is not executing from it, so it is not
		// ACTIVE: it serves nothing until the next attempt matches (#4385).
		*failures++
		mHandoffFailures.WithLabelValues(opts.Partition).Inc()
		log.Error("The handoff failed; syncing again and handing off again", "block", q, "attempt", *failures, "error", err)
		opts.State.Demote(q)
		opts.Buffer.StartCollecting()
		return false, nil
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
