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
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
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

// The gauge that says what state this node is in — 0 booting, 1 waiting, 2
// active, 3 complete — was declared here, and that was the whole of #4345a: a
// GaugeVec creates a child on the first WithLabelValues, so only a node that
// entered the join state machine ever exported the series, and absence could
// not be told from health. It lives in nodestate now and the daemon reports
// it for every partition it runs, joining or not. The join reports its own
// transitions through the same door.

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

	// anchors is where this partition's verified roots come from: the pool of
	// the partition that RECEIVES this one's anchors, because a produced
	// anchor never lives on its own partition's pool (#4301).
	anchors *anchorsrc.Source

	// authority is the validator sets this node trusts. It is seeded from
	// this node's OWN store before anything is pulled, and it moves only
	// when the spine settles: <partition>/network and /globals are spine
	// accounts, so the definition arrives verified against a root a quorum
	// signed (#4301).
	authority *anchorsrc.Authority

	machine *nodestate.Machine
	tracker *tracker.Tracker
	log     *slog.Logger

	spine        bool   // this partition's spine has been pulled and verified
	spinePending bool   // a spine fetch is held, waiting for its anchor
	round        uint64 // how many rounds have run, for the backstop's cadence
	wide         bool   // the last ledger walk could not cover (R, Q]

	// executed is the block this node's EXECUTOR last executed: the number
	// the daemon logs as lastBlock. It is read once, before anything is
	// pulled, and never again — see localBlock.
	executed uint64

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

	// ExecutedBlock is the block this node's executor last executed — the
	// number the daemon logs as lastBlock (cmd/accumulated/run/dagbft.go,
	// lastExecutedBlock). Zero means read it from the store, which is the
	// same number as long as nothing has been pulled yet.
	//
	// It is handed over so the daemon and the join cannot disagree about
	// where this node stands.
	ExecutedBlock uint64

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
		executed:  opts.ExecutedBlock,
	}

	// Read NOW, before the pull writes anything, or not at all. The store's
	// copy of this number stops being this node's the moment the ledger
	// account is pulled — see localBlock.
	if s.executed == 0 {
		n, err := readExecutedBlock(opts.Database, opts.Partition)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		s.executed = n
	}
	// The validator sets, from THIS NODE'S OWN STORE, before the pull
	// overwrites anything. This is the trust root: a signature is verified
	// against a key, not against a root, so nothing has to be fetched before
	// verification can start. A restart holds the network definition it
	// executed with and a node started from genesis holds the genesis one
	// (executor spec, "Sync", step 1; #4301).
	authority, err := anchorsrc.FromStore(opts.Database, opts.Partition)
	if err != nil {
		// A node that cannot say who the validators are cannot verify a root,
		// and a join that cannot verify a root is the defect this closes. It
		// does not proceed unverified.
		return nil, errors.UnknownError.WithFormat(
			"this node cannot say who %v's validators are, so it cannot verify anything it pulls: %w",
			opts.Partition, err)
	}

	// Producer routing. To verify THIS partition's root the node needs an
	// anchor this partition PRODUCED, and a produced anchor lives on the
	// RECEIVING partition's pool: a BVN's in dn.acme/anchors. The Directory
	// anchors to itself as well as to every BVN, so its own root is in both
	// pools under the same signatures; the join reads it from a BVN's so
	// that the copy it takes is a second partition's. What the old code
	// could not do was not obtain the Directory's root — it was check who
	// signed it (#4301).
	pool, err := anchorsrc.PoolFor(opts.Partition, authority.BvnNames())
	if err != nil {
		return nil, errors.UnknownError.WithFormat("find the pool that holds %v's anchors: %w", opts.Partition, err)
	}
	// Read from a peer that has executed those anchors — never from this
	// node, whose anchor pool is the one the pull has not filled yet (#4303).
	s.authority = authority
	s.anchors, err = anchorsrc.New(opts.Sources.Querier(pool.Identity()), pool, opts.Partition, authority)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	s.anchors.OnAnchor = track.Observe
	s.anchors.OnRefused = func(block uint64, err error) {
		// Said out loud. A source that records nothing looks exactly like a
		// network that has anchored nothing, and the difference between them
		// is the difference between a peer lying and a peer being slow.
		s.log.Info("An anchor was refused", "partition", opts.Partition, "block", block, "error", err)
	}

	// Watchable from the moment the node starts joining, and on every change.
	// The daemon has already reported this partition's state — the series
	// exists whether or not a node joins (#4345a) — so this is an update of
	// something already on the wire, not its creation.
	label := opts.Partition.String()
	if id, ok := protocol.ParsePartitionUrl(opts.Partition); ok {
		label = id
	}
	nodestate.Report(label, machine.State())
	machine.OnChange(func(ad nodestate.Advertisement) {
		nodestate.Report(label, ad.State)
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
	//
	// A failure here ends the round, never the join. Both this and the spine
	// below are reads addressed at a named peer now, so either can fail for
	// the ordinary reason that the peer it picked is restarting — and under
	// chaos that is a certainty, not a possibility. Returning an error reaches
	// join.Run, which returns, and the daemon logs "the join did not complete"
	// and abandons the goroutine: the node then collects forever, with no
	// retry, until an operator restarts it. The reads on either side of these
	// were deliberately made to log and continue; these two were missed.
	// The sets this node trusts, before anything is judged against them.
	s.refreshAuthority()

	err := s.anchors.Read(ctx)
	if err != nil {
		s.log.Info("This partition's anchors could not be read this round",
			"partition", s.partition, "error", err)
	}

	// The spine is fetched ONCE and then waited on. ModeFullSpine replays
	// every chain entry of four accounts, and a batch that cannot settle
	// this round is held and settled by settleHeld when its anchor arrives —
	// re-fetching it every round until then is a full chain replay per round
	// on a network whose Directory anchors roughly one block in six (review
	// finding 5).
	if !s.spine && !s.spinePending {
		err := s.pullSpine(ctx)
		if err != nil {
			s.log.Info("The spine could not be pulled this round; it is asked for again",
				"partition", s.partition, "error", err)
		}
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

// localBlock is the block this node's state is: the block its EXECUTOR last
// executed, remembered from before the pull started. A node that has executed
// nothing is at zero.
//
// It is NOT read from the store each time it is asked for. `<partition>/ledger`
// is an account, and it is one of the accounts the pull overwrites with the
// peer's — so after the first round the store's answer is the PEER's block and
// not this node's. Measured on the live twelve-node network of 2026-09-18: the
// joining node's own store answered 929 and the peer 946, while its executor
// was at block 76. The join therefore believed it was 17 blocks behind when it
// was 853 behind, s.wide was never set, the page diff never ran as the primary,
// and the walk covered 17 blocks instead of 853 (#4295).
//
// Nothing moves it while the join runs: a joining node collects committed
// blocks and executes none of them (join.Run, step 1), so its executor stands
// still until the handoff.
func (s *PulledState) localBlock() (uint64, error) {
	return s.executed, nil
}

// readExecutedBlock is what the daemon reads to decide whether a node must
// join at all (cmd/accumulated/run/dagbft.go, lastExecutedBlock). It is only
// this node's answer before anything has been pulled.
func readExecutedBlock(db *database.Database, partition *url.URL) (uint64, error) {
	batch := db.Begin(false)
	defer batch.Discard()
	var ledger *protocol.SystemLedger
	err := batch.Account(partition.JoinPath(protocol.Ledger)).Main().GetAs(&ledger)
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
	since    time.Time // when it was fetched, for the give-up log

	// spine says this batch is the spine's, fetched once and waited on.
	// When it finishes, the node either has a verified spine or asks for it
	// again; nothing else re-fetches it in the meantime (review finding 5).
	spine bool

	// asked is how many accounts the batch set out to settle.
	asked int
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
			continue
		}
		if h.spine {
			s.spineSettled(got, h.asked, len(missed))
		}
	}
	s.held = keep
	return pulled, refused
}

// spineSettled records what became of the one spine fetch, and moves the
// validator sets if it verified.
//
// **This is how a joining node crosses a change to the validator sets**, and
// on this line it is the only way. <partition>/network and /globals are
// spine accounts, so they arrive with a receipt that ends at a root a quorum
// of this partition's validators signed and passes through the leaf the
// pulled body hashes to; adopting them from the store afterwards is
// therefore an induction step and not a peer's word. A change never travels
// in an anchor past Vandenberg (#4301, review finding 1).
func (s *PulledState) spineSettled(got, asked, refused int) {
	s.spinePending = false
	if refused > 0 || got < asked {
		s.log.Info("The spine did not verify this round; it is asked for again",
			"partition", s.partition, "settled", got, "asked", asked, "refused", refused)
		return
	}
	s.spine = true
	s.refreshAuthority()
	s.log.Info("Pulled the spine, verified against an anchored root",
		"partition", s.partition, "accounts", got)
}

// TrustedVersion is the network definition version this join verifies
// anchors against. It moves only when refreshAuthority takes a definition
// out of verified state.
func (s *PulledState) TrustedVersion() uint64 { return s.authority.Version() }

// refreshAuthority takes the validator sets out of this node's store.
//
// **Everything in that store arrived verified**, which is what makes this an
// induction step and not a peer's word: every account the pull writes has a
// receipt that is valid, that ends at a root a quorum of this partition's
// validators signed, and that passes through the leaf the pulled body hashes
// to (pull.Verify). <partition>/network and /globals are spine accounts so
// they arrive early, but the guarantee is the pull's and not the spine's.
//
// It runs every round rather than once, because a network can change while a
// node is joining and a join that read the sets once would be stranded by
// the next change exactly as it was by the last (#4301, review finding 1).
func (s *PulledState) refreshAuthority() {
	moved, err := s.authority.UpdateFrom(s.db, s.partition)
	switch {
	case err != nil:
		s.log.Debug("The network definition could not be read from this node's store",
			"partition", s.partition, "error", err)
	case moved:
		// The window the source already read was measured against the old
		// set, so anything it refused on the way is gone unless it reads it
		// again (review finding 4).
		s.anchors.Rewind()
		s.log.Info("The validator set moved, taken from verified state",
			"partition", s.partition, "version", s.authority.Version())
	}
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
		case errors.Is(err, anchorsrc.ErrNotAnchored):
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
	//
	// SAID OUT LOUD. This is the dominant failure mode and it used to be
	// silent: the Directory anchors roughly one block in six of a BVN's, so
	// most batches are served at a block that will never be anchored and
	// five settle rounds in six end here. A node that has thrown away twelve
	// held accounts after eight hundred seconds of waiting was indisting-
	// uishable, in its log, from one that had nothing to do (#4295).
	if len(waiting) > 0 {
		s.log.Info("Pulled accounts were given up on unanchored: the directory never anchored the block they were served at",
			"partition", s.partition, "accounts", len(waiting), "rounds", h.rounds,
			"block", waiting[0].pending.Block, "waited", time.Since(h.since).Round(time.Second),
			"first", waiting[0].url)
	}
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

		h := &heldBatch{batch: s.db.Begin(true), since: time.Now()}
		for _, u := range chunk {
			srcs, partition, err := s.sourcesFor(ctx, u)
			switch {
			case errors.Is(err, errNotThisPartition):
				// Dropped, not refused: a peer named an account this store
				// must not hold, and asking again will not change that.
				s.log.Info("A named account is not this partition's and was dropped",
					"account", u, "partition", s.partition)
				continue
			case err != nil:
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
			if p.Past() {
				// The node is past this peer on this account: everything the
				// peer can give for it, the node has, and the pull took
				// nothing. There is nothing to settle and no anchor to wait
				// for, and holding it would hold a batch open for state the
				// node is never going to take.
				//
				// It is not refused either: nothing failed. The account is
				// named again next round if its leaf still differs from the
				// peer's -- enumerate.Stale names a difference in either
				// direction, so an account the node is ahead on is named
				// every round until the network passes it (#4348).
				p.Discard()
				s.log.Debug("The node is past the peer on an account; nothing was pulled",
					"account", u, "partition", s.partition)
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

// errNotThisPartition is an account that routes somewhere else. It is dropped
// and not refused: a refusal is retried for the life of the process, and this
// name is never going to become this partition's.
var errNotThisPartition = errors.NotAllowed.With("the account is not this partition's")

// sourcesFor is the peers that can answer for an account, and the partition
// they answer for. The partition matters: a receipt proves the state as of a
// block, and block numbers collide across partitions (#4308).
//
// An account that routes to another partition is refused outright. The pull
// writes into THIS partition's store, and a partition's state tree holds no
// account of another's, so such a name puts a leaf in the local BPT that no
// peer of this partition has — and the local root leaves the anchored series
// for good, however perfectly everything else is pulled. That is the same
// failure pullSpine was fixed for; the difference is that the spine's list is
// ours by construction and this one is a peer's, arriving in a block ledger
// record or a BPT page. Worse, the account verifies: it is fetched from its
// own partition's honest peers and settles against the root the Directory
// anchored for THAT partition, so nothing downstream catches it. The leaf is
// durable, so a restart does not clear it.
func (s *PulledState) sourcesFor(ctx context.Context, u *url.URL) ([]pull.Source, *url.URL, error) {
	srcs, partition, err := s.sources.For(ctx, u)
	if err != nil {
		return nil, nil, errors.UnknownError.Wrap(err)
	}
	if partition == nil {
		partition = s.partition
	}
	if !partition.Equal(s.partition) {
		return nil, nil, errNotThisPartition
	}
	return srcs, partition, nil
}

// pullSpine takes THIS PARTITION'S spine — its anchors, ledger and operators,
// with their chains — so the chains a join reads are there at all.
//
// **It is verified like everything else.** It used to be settled with Keep,
// unverified, on the rationale that it is "what the verifier reads from, and
// there is nothing to verify it against until it is there". That rationale
// was false and it cost #4301: a signature is verified against a KEY, and the
// keys come from this node's own store (anchorsrc.Authority), so nothing has
// to be pulled before verification can begin. Taking the spine on a peer's
// word made the peer the source of both the root and the state that hashes
// into it, and the whole scheme then proved only that the peer agreed with
// itself.
//
// It does NOT take the Directory's. A BVN's state tree holds no acc://dn.acme
// account: writing four of them into a BVN's store puts four leaves in its
// BPT that no peer has, so the local root differs from every root the
// Directory ever anchored for that partition, however perfectly everything
// else is pulled. A node runs the Directory alongside its BVN and the
// Directory's own join pulls the Directory's spine into the Directory's
// store, where those accounts belong.
func (s *PulledState) pullSpine(ctx context.Context) error {
	h := &heldBatch{batch: s.db.Begin(true), since: time.Now(), spine: true}
	accounts := pull.SpineAccounts(s.partition)
	for _, u := range accounts {
		srcs, partition, err := s.sourcesFor(ctx, u)
		if err != nil {
			h.batch.Discard()
			return errors.UnknownError.WithFormat("find a peer for the spine account %v: %w", u, err)
		}
		p, _, err := pull.FetchFrom(ctx, srcs, h.batch, u, pull.Options{
			Mode:      pull.ModeFullSpine,
			Verify:    s.anchors,
			Partition: partition,
		})
		if err != nil {
			h.batch.Discard()
			return errors.UnknownError.WithFormat("pull the spine account %v: %w", u, err)
		}
		if p.Past() {
			p.Discard()
			continue
		}
		h.accounts = append(h.accounts, &heldAccount{url: u, pending: p})
	}
	h.asked = len(h.accounts)

	if h.asked == 0 {
		// Everything the peers have for the spine, this node already has.
		h.batch.Discard()
		s.spine = true
		s.log.Info("The spine is already this node's", "partition", s.partition, "accounts", len(accounts))
		return nil
	}

	// Settled against the root a quorum of this partition's validators
	// signed, on the same hold the rest of the pull uses: the anchor for the
	// block a peer served at arrives a few blocks later, so the spine waits
	// for its proof rather than being taken without one.
	got, missed, done := s.settleBatch(ctx, h)
	if !done {
		s.spinePending = true
		s.held = append(s.held, h)
		return errors.NotReady.WithFormat(
			"the spine is held until its block is anchored: %d of %d settled", got, h.asked)
	}
	s.spineSettled(got, h.asked, len(missed))
	if !s.spine {
		return errors.NotReady.With("the spine did not verify")
	}
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
