// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"encoding/hex"
	"log/slog"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/enumerate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/tracker"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
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

	// staleEvery is how often the BPT page diff runs regardless of what the
	// block ledger named. It is the backstop, and a backstop that only runs
	// when the primary named nothing is not one (#4306).
	staleEvery = 8
)

// THERE IS NO SETTLE WINDOW, and that is the whole of #4362.
//
// The pull used to ask a peer for whatever block it was on, and then hold
// what came back for a few rounds hoping the Directory would anchor that
// block. It cannot work for an account that changes every block -- the
// partition's ledger, its anchors, its synthetic ledger -- because the peer
// answers at its current block, which is never anchored inside any window,
// and by the time it is the peer has moved on. A restarted node holds every
// cold account already and differs from its peers only in the hot ones, so
// the pull it needed was exactly the pull that could never settle.
//
// A pass asks at a block whose root it ALREADY HOLDS AND HAS VERIFIED, so
// every answer is settled in the round it was fetched. With nothing to wait
// for there is nothing to hold: maxSettleRounds, the held batches, the
// discard of everything an earlier round settled into a held batch (#4352)
// and the re-fetch of held accounts every round (#4353) all go with the
// wait.

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

	// pass is the pull's unit of work: one block, held fixed, with the root a
	// quorum signed for it. See syncPass.
	pass *syncPass

	// lastPass is the block of the last pass that ran to quiescence: one
	// whose root did not match, or one whose block the node could not
	// execute from because a stream had a gap. A new pass is opened above
	// it, never at it again.
	lastPass uint64

	// synced is the block this node's state is the state of: the block of a
	// pass whose root matched. Zero until one does, and cleared by Advance.
	synced uint64

	// stateAt is where the block-ledger walk starts. It is the block this
	// node's EXECUTOR reached until a pass matches, and the matched block
	// after that: the node's state IS that block's state, so the walk has
	// nothing to say below it.
	stateAt uint64

	// bus is where the definition this join pulled is published at the
	// handoff. Nil is allowed -- a test that drives the pull alone has
	// nothing to publish to -- and is the only case that does not publish.
	bus *events.Bus

	spine bool // this partition's spine has been pulled and verified
	wide  bool // the last ledger walk could not cover (R, Q]

	// executed is the block this node's EXECUTOR last executed: the number
	// the daemon logs as lastBlock. It is read once, before anything is
	// pulled, and never again — see localBlock.
	executed uint64
}

// syncPass is one pass of the pull: a block, held fixed for the whole of it,
// and the root a quorum of this partition's validators signed for that block.
//
// THE TWO ARE TAKEN TOGETHER AND NEITHER IS READ AGAIN. anchorsrc.LatestAnchor
// hands over the block and its StateTreeAnchor in one call, both already
// verified against the trusted set, and every account of the pass is asked for
// AS OF that block and settled against THAT root. Nothing in a peer's answer
// decides which root it is judged by (the #4361 threat review's F1,
// note_3870023123).
//
// Holding it fixed is what makes the pass mean anything. Taking a new
// LatestAnchor every round pulled accounts as of different blocks, and the
// local root was then a mixture no node ever held -- the state the branch was
// in when this issue was briefed, in its own commit message.
type syncPass struct {
	// block is Q: a block of this partition that a quorum signed an anchor
	// for.
	block uint64

	// root is that anchor's StateTreeAnchor: the value the local root must
	// come to equal for the node's state to be block Q's state.
	root [32]byte

	// round counts the rounds this pass has run, for the backstop's cadence
	// and for the log.
	round uint64

	// todo is what this pass has left to pull. It is seeded once, on the
	// pass's first round, and after that it is only what a round refused: an
	// account dropped once is an account the local root can never account
	// for, and the join would wait for a match that cannot come.
	//
	// THE SEED IS TAKEN ONCE, and that is what lets a pass finish. The ledger
	// walk names what the blocks in (R, Q] changed and the page diff names
	// what the peer's tree at Q holds that this node does not; neither moves
	// while the pass runs, so asking them again every round asks for the same
	// accounts for ever and the pass never goes quiet. It ran 60 rounds of
	// "asked=7 pulled=7 refused=0" that way, at one block, converging and
	// unable to say so.
	todo   []*url.URL
	seeded bool

	// rechecked says the root has been judged once and the page diff re-run
	// to find what the pass could not account for. It happens at most once
	// per pass: a second empty answer is the pass's answer.
	rechecked bool

	// barren counts consecutive rounds that pulled nothing and refused
	// something. A peer that cannot serve this block -- it has pruned it, or
	// it is behind -- is a reason to move to a newer block, not to spin.
	barren int

	// pulled counts what the pass has settled, for the log.
	pulled int

	// quiet says the last round named nothing and refused nothing, so there
	// is nothing left to pull at this block and the root can be judged.
	quiet bool
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

	// EventBus is where the pulled network definition and globals are
	// published at the handoff, as WillChangeGlobals.
	//
	// It is not optional in production and it is not decoration. The join
	// never republished what it pulled, so a joined node's committee was the
	// pre-join one until the next ON-CHAIN change -- and on this line a
	// definition moves only by a leaf pulled under a signed root (#4301
	// (c)), so "until the next on-chain change" can be for ever. One event
	// feeds three readers: the membership/submit gate, the conductor's
	// anchor gate and the adapter's committee, so all three inherited the
	// staleness together. A validator demoted while it was offline came back
	// believing it was still a member, took submissions and stranded them
	// (#4366, note_3869977850).
	EventBus *events.Bus

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
		bus:       opts.EventBus,
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
	s.stateAt = s.executed
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

// Pull runs one round of the current pass, opening a pass when none is open
// (executor spec, "Sync", §3 and §5).
//
// A pass is one block held fixed. Everything the round asks for is asked for
// as of that block and settled against the root taken with it, so nothing is
// held and nothing waits. When a round names nothing and refuses nothing the
// pass is quiet and Matched judges the root.
func (s *PulledState) Pull(ctx context.Context) error {
	// The sets this node trusts, before anything is judged against them.
	//
	// A failure below ends the round, never the join. Every read here is
	// addressed at a named peer, so any of them can fail for the ordinary
	// reason that the peer it picked is restarting -- under chaos a
	// certainty. Returning an error reaches join.Run, which returns, and the
	// node then collects for ever with no retry.
	if s.synced != 0 {
		// The state is a block's state and the question is no longer what to
		// pull but whether that block can be executed from. Advance says
		// otherwise.
		return nil
	}
	s.refreshAuthority()

	err := s.anchors.Read(ctx)
	if err != nil {
		s.log.Info("This partition's anchors could not be read this round",
			"partition", s.partition, "error", err)
	}

	p, err := s.openPass(ctx)
	if p == nil {
		return err
	}
	p.round++

	if !p.seeded {
		s.seed(ctx, p)
		p.seeded = true
	}

	accounts := dedupe(p.todo)
	p.todo = nil
	if len(accounts) == 0 {
		// Nothing left to pull at this block, so the root can be judged.
		p.quiet = true
		s.log.Info("A sync pass is quiet: everything this block names has been pulled",
			"partition", s.partition, "block", p.block, "rounds", p.round, "pulled", p.pulled)
		return nil
	}

	got, missed := s.fetch(ctx, p, accounts)
	p.pulled += got
	p.todo = missed
	p.quiet = len(missed) == 0
	if got == 0 && len(missed) > 0 {
		p.barren++
	} else {
		p.barren = 0
	}
	s.log.Info("Pulled the accounts this block names", "partition", s.partition,
		"block", p.block, "round", p.round, "asked", len(accounts), "pulled", got,
		"refused", len(missed), "barren", p.barren)

	if p.barren >= maxBarrenRounds {
		// No peer could serve this block for anything that is left. That is
		// this block going out of reach -- a peer prunes what it retains --
		// and not a reason to keep asking about it. A newer block is asked
		// about instead.
		s.log.Info("No peer could serve this block for what is left; a later block is asked for instead",
			"partition", s.partition, "block", p.block, "left", len(missed), "rounds", p.round)
		s.lastPass = p.block
		s.pass = nil
	}
	return nil
}

// maxBarrenRounds is how many consecutive rounds may pull nothing and refuse
// something before the pass gives up on its block. A block a peer cannot
// serve is a block out of its retained range, and asking again does not bring
// it back.
const maxBarrenRounds = 4

// seed is what the pass asks for, taken once, at the pass's block.
//
// Two sources, and the page diff is not a fallback. The ledger walk names what
// the blocks in (R, Q] changed, which is exact and cheap for a restart; the
// page diff names every leaf of the peer's tree at Q this node does not hold
// or does not agree with, which covers what the walk cannot -- an account
// whose body moved with no chain of its own moving, and the whole tree for a
// node joining from further back than a walk is worth. Running the diff only
// when the walk named nothing made it unreachable, because one name that can
// never be satisfied keeps the set non-empty for the life of the process
// (#4306).
func (s *PulledState) seed(ctx context.Context, p *syncPass) {
	// The block ledger first, because it says how long every chain it names
	// was at this block -- which the spine pull needs as much as any other.
	changed, err := s.changedAccounts(ctx, p.block)
	if err != nil {
		// A peer that cannot serve its block ledger this second is not a
		// reason to abandon the join: the page diff covers the pass.
		s.log.Info("The block ledger could not be read", "partition", s.partition,
			"block", p.block, "error", err)
	}
	p.todo = append(p.todo, changed...)

	// Then the spine: the accounts the verifier and the executor read from,
	// with their chains replayed.
	if !s.spine {
		got, missed := s.pullSpine(ctx, p)
		p.pulled += got
		p.todo = append(p.todo, missed...)
	}

	stale, err := s.staleAccounts(ctx, p.block)
	if err != nil {
		s.log.Info("The peer's BPT could not be paged", "partition", s.partition,
			"block", p.block, "error", err)
	}
	p.todo = append(p.todo, stale...)

	s.log.Info("A sync pass has its set", "partition", s.partition, "block", p.block,
		"fromTheBlockLedger", len(changed), "fromThePageDiff", len(stale), "wide", s.wide)
}

// openPass returns the pass in progress, opening one when there is none.
//
// A pass is opened at the highest block a quorum of this partition's
// validators has signed an anchor for, WITH the root that anchor carries.
// Both come out of one call and neither is read again: the block an account is
// settled against is the block this node asked at, held with its verified
// StateTreeAnchor before the request is made (#4361's F1).
//
// It refuses to open one in three cases, each a wait and not a failure: no
// anchor has been verified yet; the anchored block is below the block this
// node's own executor reached, so the pull would be asked to walk the node
// backwards onto state peers may already have pruned; and the anchored block
// is one a pass has already run to quiescence at without matching, which is a
// block to move past rather than to try again.
func (s *PulledState) openPass(ctx context.Context) (*syncPass, error) {
	if s.pass != nil {
		return s.pass, nil
	}

	q, root, err := s.anchors.LatestAnchor(ctx)
	if err != nil {
		s.log.Info("The latest anchored block could not be read this round",
			"partition", s.partition, "error", err)
		return nil, nil
	}
	switch {
	case q == 0 || root == ([32]byte{}):
		s.log.Info("No anchor of this partition has been verified yet; nothing can be asked for at a block",
			"partition", s.partition)
		return nil, nil
	case q < s.executed:
		s.log.Info("The latest anchored block is behind this node's own; waiting for the anchors to catch up",
			"partition", s.partition, "anchored", q, "executed", s.executed)
		return nil, nil
	case q <= s.lastPass:
		s.log.Info("Waiting for an anchor above the block the last pass ran at",
			"partition", s.partition, "anchored", q, "lastPass", s.lastPass)
		return nil, nil
	}

	s.pass = &syncPass{block: q, root: root}
	s.log.Info("A sync pass opened at an anchored block", "partition", s.partition,
		"block", q, "root", logHash(root), "executed", s.executed)
	return s.pass, nil
}

// logHash is the first four bytes of a root, for a log line.
func logHash(h [32]byte) string { return hex.EncodeToString(h[:4]) }

// changedAccounts is the union of what the blocks in (R, Q] changed, where R
// is the block this node's EXECUTOR reached and Q is the pass's block: the
// block ledger's answer, not the envelopes' (executor spec, "Sync", §3).
//
// Q IS THE PASS'S BLOCK, not a number read from a peer's ledger. Asking a
// peer where it stands and walking to there named accounts the anchored root
// of the pass does not cover, and it made the end of the span a peer's word
// -- which is the same class of mistake as settling against a block the
// answer carries.
func (s *PulledState) changedAccounts(ctx context.Context, through uint64) ([]*url.URL, error) {
	s.wide = false

	r := s.stateAt
	if through <= r {
		// This node's state is at or past the pass's block. There is nothing
		// for the walk to say; the backstop decides the pass.
		return nil, nil
	}

	// A span wider than a walk is worth is the page diff's: it answers the
	// same question in one scan, and that is the case of a node joining from
	// genesis.
	from := r
	if through-from > MaxLedgerSpan {
		s.wide = true
		from = through - MaxLedgerSpan
	}

	q := api.Querier2{Querier: s.sources.Querier(s.partition)}
	entries, err := blockLedger(ctx, q, s.partition, from, through)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if s.wide {
		// The accounts the window named are a fraction of what a node this
		// far behind must pull, so the page diff is the set.
		return nil, nil
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

// staleAccounts is the BPT page diff: the accounts whose leaf this node does
// not hold, or holds and does not agree with. Nothing is written — a leaf
// taken from a peer's word would make the local root the peer's, and the local
// root is what the tracker matches (package enumerate).
//
// It is read AS OF THE PASS'S BLOCK (#4361's BptPageQuery.ForHeight). A page
// from the peer's current tree is a difference against a moving target, so a
// pass that holds one block fixed would be named new accounts for ever and
// never go quiet; at the pass's block the page is the tree the anchored root
// commits to, so the difference IS the set the node must pull to reach that
// root, and it shrinks to nothing.
func (s *PulledState) staleAccounts(ctx context.Context, at uint64) ([]*url.URL, error) {
	res, err := s.pageDiff(ctx, at)
	if res == nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if err != nil {
		return res.Stale, errors.UnknownError.WithFormat("find what the peer holds that this node does not: %w", err)
	}
	return res.Stale, nil
}

// pageDiff is the raw page diff at a block: what the peer's tree holds at that
// block that this node does not hold or does not agree with.
func (s *PulledState) pageDiff(ctx context.Context, at uint64) (*enumerate.Result, error) {
	batch := s.db.Begin(false)
	defer batch.Discard()
	return enumerate.Run(ctx, api.Querier2{Querier: s.sources.Querier(s.partition)},
		s.partition, batch, enumerate.Options{AtBlock: at})
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

// fetch pulls the accounts, in chunks, and settles each one in the round it
// was fetched against the root the pass holds.
//
// There is no hold and no second round. The pass asked for the state as of a
// block whose root a quorum signed and which this node verified before it
// asked, so the value every answer is judged against is already in hand: an
// account either hashes into it now or it is refused and asked for again.
func (s *PulledState) fetch(ctx context.Context, p *syncPass, accounts []*url.URL) (int, []*url.URL) {
	pulled := 0
	var refused []*url.URL

	for len(accounts) > 0 && ctx.Err() == nil {
		n := pullChunk
		if n > len(accounts) {
			n = len(accounts)
		}
		chunk := accounts[:n]
		accounts = accounts[n:]

		batch := s.db.Begin(true)
		got := 0
		for _, u := range chunk {
			ok, err := s.fetchOne(ctx, p, batch, u, pull.ModeStateOnly)
			switch {
			case errors.Is(err, errNotThisPartition):
				// Dropped, not refused: a peer named an account this store
				// must not hold, and asking again will not change that.
				s.log.Info("A named account is not this partition's and was dropped",
					"account", u, "partition", s.partition)
			case err != nil:
				refused = append(refused, u)
			case ok:
				got++
			}
		}
		if got == 0 {
			batch.Discard()
			continue
		}
		if !s.commit(batch) {
			// Nothing reached the store, so nothing is accounted for: every
			// account in the chunk is asked for again.
			for _, u := range chunk {
				refused = append(refused, u)
			}
			continue
		}
		pulled += got
	}
	return pulled, refused
}

// fetchOne pulls one account as of the pass's block and settles it against the
// pass's root, writing into the caller's batch. It reports whether anything
// was written: an account this node is already past the peer on is a complete
// and empty pull, not a refusal.
func (s *PulledState) fetchOne(ctx context.Context, p *syncPass, batch *database.Batch, u *url.URL, mode pull.Mode) (bool, error) {
	srcs, partition, err := s.sourcesFor(ctx, u)
	if err != nil {
		if errors.Is(err, errNotThisPartition) {
			return false, err
		}
		s.log.Info("No peer could be found for an account", "account", u, "error", err)
		return false, err
	}

	pending, _, err := pull.FetchFrom(ctx, srcs, batch, u, pull.Options{
		Mode:      mode,
		Verify:    s.anchors,
		Partition: partition,
		AtBlock:   p.block,
	})
	if err != nil {
		s.log.Info("An account could not be pulled", "account", u,
			"block", p.block, "error", err)
		return false, err
	}

	// Settled against the root the pass holds -- not against a root looked up
	// from a block the answer named (#4361's F1).
	err = pending.Settle(p.root)
	if err != nil {
		s.log.Info("A pulled account did not verify against the anchored root",
			"account", u, "block", p.block, "root", logHash(p.root), "error", err)
		return false, err
	}
	return true, nil
}

// commit writes the state tree and then the batch, and says whether both
// held.
//
// The BPT, then the commit. Batch.Commit commits the BPT store and never
// calls Account.putBpt, so a perfectly pulled account would leave the local
// root exactly where it was and the pass could never match it (#4305).
func (s *PulledState) commit(batch *database.Batch) bool {
	if err := batch.UpdateBPT(); err != nil {
		s.log.Info("The state tree could not be updated", "partition", s.partition, "error", err)
		batch.Discard()
		return false
	}
	if err := batch.Commit(); err != nil {
		s.log.Info("What was pulled could not be committed", "partition", s.partition, "error", err)
		return false
	}
	return true
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
func (s *PulledState) pullSpine(ctx context.Context, p *syncPass) (int, []*url.URL) {
	accounts := pull.SpineAccounts(s.partition)
	batch := s.db.Begin(true)
	got, asked := 0, 0
	var refused []*url.URL
	for _, u := range accounts {
		ok, err := s.fetchOne(ctx, p, batch, u, pull.ModeFullSpine)
		switch {
		case err != nil:
			refused = append(refused, u)
		case ok:
			asked++
			got++
		}
	}
	if got == 0 {
		batch.Discard()
		if len(refused) == 0 {
			// Everything the peers have for the spine, this node already has.
			s.spine = true
			s.refreshAuthority()
			s.log.Info("The spine is already this node's", "partition", s.partition, "accounts", len(accounts))
		}
		s.spineSettled(0, asked, len(refused))
		return 0, refused
	}
	if !s.commit(batch) {
		return 0, accounts
	}
	s.spineSettled(got, asked, len(refused))
	return got, refused
}

// Matched reports the block this node's state is the state of, and whether it
// has reached one (executor spec, "Sync", §5).
//
// It is answered by ONE comparison: the local BPT root against the root the
// pass took with its block. Not against the set of every anchor this node has
// ever observed -- that is what lets a peer decide which anchored root it is
// judged by. The threat review's sequence: the peer answers at an older
// anchored B', every account verifies because the receipt genuinely ends at a
// root the joiner holds, the local root becomes root(B'), a matcher that
// accepts any observed root promotes at B', and the node then executes B'+1,
// a block it never collected (note_3870023123, F1). A pass asks at a block it
// chose and is answered at that block or not at all.
//
// A pass that has gone quiet and does not match is a pass that could not
// account for something. It says what: the accounts the peer's tree holds at
// that block that this node does not hold or does not agree with. Then the
// pass is closed and a new one is opened above it -- never at the same block
// again, because the block is not what was wrong.
func (s *PulledState) Matched(ctx context.Context) (uint64, bool, error) {
	if s.synced != 0 {
		return s.synced, true, nil
	}
	p := s.pass
	if p == nil || !p.quiet {
		return 0, false, nil
	}

	batch := s.db.Begin(false)
	local, err := batch.GetBptRootHash()
	batch.Discard()
	if err != nil {
		return 0, false, errors.UnknownError.WithFormat("read this node's own root: %w", err)
	}

	if local == p.root {
		// SYNCED, NOT PROMOTED. The node holds block Q's state and still
		// cannot serve for it: it has executed nothing, and whether it may
		// execute Q+1 is the next question. Promoting here would make a node
		// that is about to find a gap answer as though it were running
		// (#4295), and would be the first half of executing from a state
		// the node cannot pair a block with.
		s.synced = p.block
		s.stateAt = p.block
		s.log.Info("The state this node holds is the state block Q is",
			"partition", s.partition, "block", p.block, "root", logHash(local),
			"rounds", p.round, "pulled", p.pulled)
		return p.block, true, nil
	}

	// Judged once, and the page diff re-run before the pass is given up on.
	// The pass's set was taken before anything was pulled; an account whose
	// leaf only moved because another account's pull changed what it hashes
	// over, or one a page the peer served late named, is named now and the
	// pass finishes the job rather than starting over at a new block.
	if !p.rechecked {
		p.rechecked = true
		if res, err := s.pageDiff(ctx, p.block); res != nil && len(res.Stale) > 0 {
			p.todo = res.Stale
			p.quiet = false
			s.log.Info("The root does not match yet and the page diff names more to pull",
				"partition", s.partition, "block", p.block, "local", logHash(local),
				"anchored", logHash(p.root), "more", len(res.Stale), "error", err)
			return 0, false, nil
		}
	}

	s.nameTheDifference(ctx, p, local)
	s.lastPass = p.block
	s.pass = nil
	return 0, false, nil
}

// nameTheDifference says what a quiet pass could not account for.
//
// A page carries no proof, so a peer can omit a leaf, and the root failing to
// match is the only detector there is -- which is why a mismatch must name
// what it could not account for (executor spec, "Sync", §2). Two kinds are
// told apart, because they mean different things: an account whose leaf this
// node does not hold at all is a pull that has not run, and one it holds a
// different leaf for is a pull that ran and did not reproduce the leaf.
func (s *PulledState) nameTheDifference(ctx context.Context, p *syncPass, local [32]byte) {
	res, err := s.pageDiff(ctx, p.block)
	if res == nil {
		s.log.Info("The state this node holds is not block Q's, and the difference could not be read",
			"partition", s.partition, "block", p.block,
			"local", logHash(local), "anchored", logHash(p.root), "error", err)
		return
	}
	names := res.Stale
	if len(names) > 8 {
		names = names[:8]
	}
	s.log.Info("The state this node holds is not block Q's: a pass ran to quiescence and the root does not match",
		"partition", s.partition, "block", p.block,
		"local", logHash(local), "anchored", logHash(p.root),
		"rounds", p.round, "pulled", p.pulled,
		"leavesSeen", res.LeavesSeen, "unaccountedFor", len(res.Stale),
		"heldAndDisagreeing", len(res.Disagree), "first", names, "error", err)
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

// Advance gives up executing from the block the state is at, and moves the
// sync to a later anchored block (executor spec, "Sync", §4).
//
// It is what a gap is answered with. The node has block B's state and the
// block after it carries an entry number the node holds nothing at, from
// before it was listening; executing anyway is #4290. So it does not execute.
// It keeps what it collected — nothing here drops a staged entry — and the
// next pass runs at the next anchored block above B, whose pulled ledgers say
// what the peers actually ran. Everything the node lacks below that is then
// at or under the new Delivered and is not a gap any more, and the question is
// asked again of the block after it.
//
// The spec says "advance the sync one block at a time". A block is asked for
// AT an anchored block, because only an anchored block has a root this node
// can check an answer against, and this line anchors a partition roughly one
// block in three to six — so the next anchored block above B is where the
// sync can actually go. It is never behind B+1, so it is never less than the
// spec's step.
func (s *PulledState) Advance() {
	if s.synced == 0 {
		return
	}
	s.log.Info("The block after the state cannot be executed; the sync advances past it",
		"partition", s.partition, "block", s.synced)
	s.lastPass = s.synced
	s.synced = 0
	s.pass = nil
}

// Handoff publishes what the join pulled and records that this node is
// executing from block q. It runs before the first block is executed.
//
// THE DEFINITION THIS NODE PULLED IS PUBLISHED HERE, and on this line that is
// the only way a definition ever reaches the rest of the process while a node
// is running: past Vandenberg an anchor does not carry a change to the
// validator sets, so a joined node that did not republish kept the committee
// it had before it went away until the next on-chain change (#4301 (c),
// note_3869977850). Three readers take the one event — the membership and
// submit gate, the conductor's anchor gate and the adapter's committee — so
// all three moved together, or none of them did.
//
// It publishes from THIS NODE'S OWN STORE, which is an induction step and not
// a peer's word: <partition>/network and /globals are spine accounts, so what
// is in the store arrived with a receipt that is valid, that ends at the root
// a quorum signed for the block this node asked at, and that passes through
// the leaf the pulled body hashes to.
func (s *PulledState) Handoff(ctx context.Context, q uint64) error {
	g := new(core.GlobalValues)
	err := g.Load(s.partition, func(account *url.URL, target interface{}) error {
		batch := s.db.Begin(false)
		defer batch.Discard()
		return batch.Account(account).Main().GetAs(target)
	})
	if err != nil {
		return errors.UnknownError.WithFormat(
			"read the network definition this join pulled: %w", err)
	}

	batch := s.db.Begin(false)
	root, err := batch.GetBptRootHash()
	batch.Discard()
	if err != nil {
		return errors.UnknownError.WithFormat("read this node's own root: %w", err)
	}

	if s.bus != nil {
		err = s.bus.Publish(events.WillChangeGlobals{New: g.Copy()})
		if err != nil {
			return errors.UnknownError.WithFormat(
				"publish the network definition this join pulled: %w", err)
		}
	}

	if !s.machine.PromoteToActive(root, q) {
		return errors.Conflict.WithFormat("%v is %v, not joining", s.partition, s.machine.State())
	}
	s.log.Info("The definition this join pulled is published, and this node is executing",
		"partition", s.partition, "block", q, "root", logHash(root),
		"networkVersion", g.Network.Version, "published", s.bus != nil)
	return nil
}
