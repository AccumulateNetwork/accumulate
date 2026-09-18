// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package join brings a node from "listening" to "executing" (executor spec,
// "Sync"): it takes a running validator's staging, keeps it current from the
// blocks consensus commits, pulls the state those blocks name, and hands off
// to block production at the first block after the local root matches a root
// the Directory anchored.
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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
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
	// holds, so the blocks since the snapshot are no longer all in hand.
	BufferOverrun() bool

	// NamedAccounts is every account the blocks collected since the last call
	// name — what the pull must fetch for those blocks (executor spec,
	// "Sync", step 3). Draining it, so a round pulls what the blocks of that
	// round named.
	NamedAccounts() []*url.URL

	// ApplyStaging runs load — the executor taking a peer's staging — and
	// then applies every block buffered since this node started collecting to
	// THAT staging, in order, and each new one as it arrives. The two are one
	// step because the order is the spec's: staging comes from a validator as
	// of its block P, and the blocks after P are applied to it (executor
	// spec, "Sync", step 2). A node that collected into its own staging first
	// would have nothing to load into.
	ApplyStaging(load func() error) error

	// Handoff leaves collecting mode at block q and produces the buffered
	// groups from q + 1 in order.
	Handoff(q uint64) error
}

// A Stage is the executor's staging half of a join (#4292).
type Stage interface {
	// LoadStaging takes a peer's staging into this node's own.
	LoadStaging(*private.StagingSnapshot) error

	// SettleStaging brings staging to the block the pulled state is.
	SettleStagingAt(q uint64) error
}

// A State is the state half: it pulls what the node lacks and says when the
// local root equals a root the Directory anchored (#4293).
type State interface {
	// Pull fetches what the node lacks of the accounts named, verified
	// against the anchored root. Called once per round with the accounts the
	// blocks collected since the last round named.
	Pull(ctx context.Context, accounts []*url.URL) error

	// Matched reports the block whose anchored root the local root equals,
	// and whether it has been reached.
	Matched(ctx context.Context) (uint64, bool, error)
}

// A Peers finds the partition's other validators and their private API.
type Peers interface {
	// Validators lists the nodes serving this partition's sequencer, in no
	// particular order.
	Validators(ctx context.Context) ([]*api.FindServiceResult, error)

	// Staging is the private API addressed to one node.
	Staging(peer *api.FindServiceResult) private.StagingSnapshotter
}

// Options are what a join needs to run.
type Options struct {
	Partition string
	Buffer    Buffer
	Stage     Stage
	State     State
	Peers     Peers
	Logger    *slog.Logger

	// Retry is how long to wait before asking again when no peer can serve a
	// snapshot, or when the pull has not reached an anchored root.
	Retry time.Duration

	// Rounds is how many times every validator is asked for staging before
	// the join answers ErrNoPeerHasStaging. Zero means DefaultRounds.
	Rounds int
}

// DefaultRetry is how long a join waits before asking again.
const DefaultRetry = 2 * time.Second

// DefaultRounds is how many times a join asks every validator for staging
// before it concludes that none of them has any to give.
const DefaultRounds = 10

// An Outcome is how a join ended. It is a value and not an error code,
// because the one the caller acts on — no peer had staging to give — must be
// told apart from every other NotReady this node meets, and NotReady is what
// an anchor that has not reached its quorum yet, an account the Directory has
// not anchored yet, and a peer that is busy all return. Matching on the code
// would route a routine "not yet" into "execute from where you stand", which
// pairs a peer's staging with this node's older state: the divergence the
// join exists to prevent (#4290).
type Outcome int

const (
	// Joined: the node took a peer's staging, pulled the state, matched the
	// root and handed off. It is executing.
	Joined Outcome = iota

	// NoPeerHasStaging: every validator of the partition was asked and none
	// could serve any. On a network that restarted as a whole that is the
	// true answer — every node's staging is empty — so there is nothing to
	// take and nothing to be exact about, and the caller may execute from
	// where it stands. A node restarting alone gets a real answer instead.
	NoPeerHasStaging
)

// Run joins the partition. It returns when the node has handed off to block
// production, or when the context is cancelled.
//
// The order is the spec's, and each step is refused rather than guessed at:
//
//  1. listen — collecting mode, so no committed block is executed and every
//     one is kept;
//  2. take staging from a validator, as of that validator's last committed
//     block P. Collecting starts first, so every block above P is one this
//     node has collected and everything at or below P is in the snapshot:
//     there is no hole between them, and nothing to compare;
//  3. pull the state the buffered blocks name;
//  4. when the local root equals the anchored root of a block Q at or above
//     P, settle staging at Q and hand off: block Q + 1 executes from the
//     buffer, as any node executes a block.
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

	// The join starts over from here whenever the buffer overruns: the blocks
	// since the snapshot are no longer all in hand, so nothing taken before
	// is usable and a newer snapshot is taken instead.
	for {
		opts.Buffer.StartCollecting()

		// Step 2, retried: a peer that is itself joining refuses (NotReady,
		// #4295), and a peer too busy to finish a paged read says so. Either
		// means ask the next validator, not give up. When every validator
		// has been asked and none could answer, that IS the answer.
		p, taken, err := takeStaging(ctx, opts, log, retry)
		if err != nil {
			return Joined, errors.UnknownError.Wrap(err)
		}
		if !taken {
			log.Info("No validator of this partition could serve its staging")
			return NoPeerHasStaging, nil
		}
		log.Info("Staging taken from a peer", "block", p)

		// Steps 3 and 4: pull until the root matches, and hand off at Q.
		restart := false
		for !restart {
			if err := ctx.Err(); err != nil {
				return Joined, errors.UnknownError.Wrap(err)
			}
			if opts.Buffer.BufferOverrun() {
				// The blocks since P are no longer all in hand, so the join
				// cannot be exact. Start again from a newer snapshot.
				log.Info("The join buffer overran; starting again from a newer snapshot")
				restart = true
				break
			}

			q, ok, err := opts.State.Matched(ctx)
			if err != nil {
				return Joined, errors.UnknownError.WithFormat("match the anchored root: %w", err)
			}
			if ok && q >= p {
				err = opts.Stage.SettleStagingAt(q)
				if err != nil {
					return Joined, errors.UnknownError.WithFormat("settle staging at %d: %w", q, err)
				}
				err = opts.Buffer.Handoff(q)
				switch {
				case err == nil:
					log.Info("Joined", "block", q, "snapshotBlock", p)
					return Joined, nil
				case errors.Is(err, errors.NotReady):
					// The pull ran ahead of the blocks consensus has
					// delivered: handing off now would give the blocks still
					// to arrive the wrong numbers. Wait for them.
					log.Info("The state is ahead of the blocks collected so far; waiting", "block", q, "error", err)
				default:
					return Joined, errors.UnknownError.WithFormat("hand off at %d: %w", q, err)
				}
			} else if ok {
				// The root matched a block below the staging this node took.
				// Staging as of P and state as of Q < P is a mixture no node
				// ever held; keep pulling until the state reaches P.
				log.Info("The root matched below the staging taken; still pulling", "matched", q, "snapshotBlock", p)
			}

			err = opts.State.Pull(ctx, opts.Buffer.NamedAccounts())
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
}

// takeStaging asks the partition's validators, one by one, for their staging,
// and loads the first answer that is usable. A validator that refuses, that
// cannot be read, or that has executed no block itself is passed over: that
// is that node's condition, not an answer about the snapshot, and the next
// validator is asked.
func takeStaging(ctx context.Context, opts Options, log *slog.Logger, retry time.Duration) (uint64, bool, error) {
	rounds := opts.Rounds
	if rounds <= 0 {
		rounds = DefaultRounds
	}
	for attempt := 0; ; attempt++ {
		if attempt >= rounds {
			return 0, false, nil
		}
		if err := ctx.Err(); err != nil {
			return 0, false, errors.UnknownError.Wrap(err)
		}
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return 0, false, errors.UnknownError.Wrap(ctx.Err())
			case <-time.After(retry):
			}
		}

		peers, err := opts.Peers.Validators(ctx)
		if err != nil {
			log.Info("Cannot find this partition's validators yet", "error", err)
			continue
		}
		for _, peer := range peers {
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
			return snap.Block, true, nil
		}
	}
}
