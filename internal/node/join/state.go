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
	// passLimit is how many accounts one pass fetches. Every fetched account
	// holds an open child batch until the pass settles (pull.MaxHeld), so it
	// bounds what a pass holds in memory; what is left over is asked for in
	// the next pass.
	passLimit = pull.MaxHeld / 2

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

	spine bool   // this partition's spine has been pulled and verified
	round uint64 // how many rounds have fetched, for the backstop's cadence
	wide  bool   // the last ledger walk could not cover (localBlock, Q]

	// executed is the block this node's EXECUTOR last executed: the number
	// the daemon logs as lastBlock. It is read once, before anything is
	// pulled, and never from the store again — see localBlock. After the
	// handoff it is the last block whose root matched its anchor (HandedOff,
	// Diverged).
	executed uint64

	// synced is the block the pulled state is at: the ledger index of the
	// last pass whose proven root the whole local root equals. It moves as
	// the join converges, while executed stands still, and it is where the
	// ledger walk starts once it is past executed — see localBlock. The
	// handoff clears it: from there executed is the block the state is at.
	synced uint64

	// pass is what has been fetched and not yet settled: the peer's CURRENT
	// state, held until the root its receipts end at is proven — it equals
	// the StateTreeAnchor of a verified signed anchor, or the bpt chain's
	// history from one such root to it hashes into the root chain anchor a
	// later one signs (anchorsrc.ProveRoot). Nothing new is fetched while it
	// is held, so a pass is never committed over a newer one.
	pass *pass

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

// Sources is what this join pulls from: the peers it asks, which never include
// this node (#4303). Exposed so a test can check the set the join itself uses
// rather than a second one built beside it.
func (s *PulledState) Sources() Sources { return s.sources }

// Pull fetches what the blocks changed, and this partition's spine the first
// time, as ONE PASS: the peer's current state, held until the root its
// receipts end at is proven, then written whole. Every account is verified
// against that root; one that cannot be verified is not written, and is asked
// for again in the next pass.
func (s *PulledState) Pull(ctx context.Context) error {
	// The anchors first: they are what everything else is verified against.
	//
	// A failure here ends the round, never the join. Every read is addressed
	// at a named peer, so any of them can fail for the ordinary reason that
	// the peer it picked is restarting — and under chaos that is a certainty.
	// Returning an error reaches join.Run, which returns, and the daemon then
	// abandons the join for good.
	//
	// The sets this node trusts, before anything is judged against them.
	s.refreshAuthority()

	err := s.anchors.Read(ctx)
	if err != nil {
		s.log.Info("This partition's anchors could not be read this round",
			"partition", s.partition, "error", err)
	}

	// A pass that is held settles first, and nothing new is fetched while it
	// waits: the state it holds is the peer's at one root, and a second pass
	// fetched now would be at a later one.
	if s.pass != nil {
		s.settlePass(ctx)
		return nil
	}

	// The cadence counts the rounds that fetch, and only those. A round that
	// settles returns above without reaching the page diff, so a count of
	// every round is decided only on the fetching ones, and a pass that
	// always settles in a number of rounds sharing a factor with staleEvery
	// makes those miss every multiple of it for ever (#4395).
	s.round++

	// What the last pass could not pull is asked for again, with whatever
	// this round's blocks changed.
	accounts := s.refused
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

	s.fetchPass(ctx, dedupe(accounts))
	if s.pass != nil {
		s.settlePass(ctx)
	}
	return nil
}

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
// executed, remembered from before the pull started, until a pass has synced
// the state past it. A node that has executed nothing and synced nothing is at
// zero.
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
// It is the executor's block OR the block the pulled state is synced to,
// whichever is later. A joining node's executor stands still until the
// handoff (join.Run, step 1), but its state does not: every pass whose proven
// root the whole local root equals puts the state at that root's block, and
// every block after it is all the walk has to cover. Measuring from the
// executor's block alone made every round of a join longer than MaxLedgerSpan
// blocks wide, and the join ran on the page diff for the rest of its life
// (#4356). A state that is only partly pulled past synced is still covered:
// the walk from synced names everything since.
//
// After the handoff HandedOff and Diverged move executed to the last block
// whose root matched its anchor, which is where a node that syncs again
// starts from; HandedOff clears synced so that it cannot outrun them.
func (s *PulledState) localBlock() (uint64, error) {
	if s.synced > s.executed {
		return s.synced, nil
	}
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

// pass is one fetch: the accounts it pulled and the batch they were pulled
// into. Nothing in it is written until every account in it is settled or
// refused, because a Pending writes into this batch when it settles and a
// batch cannot be committed while a child of it is open.
type pass struct {
	batch    *database.Batch
	accounts []*heldAccount
	since    time.Time

	// spine says the pass carries this partition's spine, and spineFailed
	// that one of its accounts could not be fetched, so the pass cannot
	// verify the spine whatever else it settles.
	spine       bool
	spineFailed bool
}

// heldAccount is one fetched account waiting for its root to be proven.
type heldAccount struct {
	url     *url.URL
	pending *pull.Pending
	spine   bool
}

// settlePass settles the held pass once the root its receipts end at is
// proven, and writes what verified.
//
// **The accounts a peer serves are current, and the root is proven by a
// signed anchor and the history.** A peer serves an account as of its current
// block, with a receipt to its current BPT root. That root is proven when it
// EQUALS the StateTreeAnchor of an anchor a quorum signed and this node
// verified, or when the bpt chain's history from one such root to it hashes
// into the root chain anchor a later verified anchor signs, and in no other
// way (anchorsrc.ProveRoot): the anchor of block N carries the root block N
// committed, which is the root the peer is current at while its ledger says
// N, and block N+1 records it on the bpt chain. So a pass served at block N
// is proven once any anchor after N is verified, whether or not block N sent
// one -- under load every block does; idle, only a heartbeat block does, and
// the passes served between heartbeats are proven by the next one.
//
// **A pass is held only while waiting can end.** The one wait is a root no
// verified anchor reaches yet, and the next anchor may. A root the history
// has PASSED -- an anchor of a later block is verified and the bpt chain does
// not record it -- is not a wait, because no anchor to come changes it, so
// the pass is dropped and fetched again, and no count of rounds is involved.
//
// **One pass is one root.** The peers move while a pass is fetched, so its
// accounts can end at different roots, each of them true. Written together
// they are a state no block ever had, which the local root can match nothing
// with. The root most of the pass ends at is the pass; the rest are fetched
// again, as are accounts served with no receipt at all.
func (s *PulledState) settlePass(ctx context.Context) {
	p := s.pass
	asked := len(p.accounts)
	root, servedAt := p.oneRoot(func(a *heldAccount, why string) {
		s.log.Debug("A pulled account is fetched again", "account", a.url, "reason", why)
		a.pending.Discard()
		if a.spine {
			p.spineFailed = true
		} else {
			s.refused = append(s.refused, a.url)
		}
	})
	if len(p.accounts) < asked {
		s.log.Info("Part of a pass was served at another root, or at none, and is fetched again",
			"partition", s.partition, "kept", len(p.accounts), "again", asked-len(p.accounts))
	}
	if len(p.accounts) == 0 {
		s.pass = nil
		p.batch.Discard()
		return
	}

	ok, err := s.anchors.ProveRoot(ctx, s.sources.Querier(s.partition), root, servedAt)
	if err != nil {
		// Nothing to come will prove this root. What was served at it is not
		// written; the pass is asked for again, and the peers are asked in
		// rotation, so the next fetch starts at another one.
		s.log.Info("The root a pass was served at did not prove; the pass is asked for again",
			"partition", s.partition, "root", fmt.Sprintf("%x", root[:4]), "error", err)
		s.dropPass()
		return
	}
	if !ok {
		// No verified anchor carries it yet, and the next one may. Held.
		s.log.Debug("The root a pass was served at is not proven yet",
			"partition", s.partition, "root", fmt.Sprintf("%x", root[:4]),
			"waited", time.Since(p.since).Round(time.Millisecond))
		return
	}
	s.pass = nil
	pulled := 0
	for _, a := range p.accounts {
		if err := a.pending.Settle(a.pending.Root()); err != nil {
			s.log.Info("A pulled account did not verify", "account", a.url, "error", err)
			if a.spine {
				// Asked for again with the spine, whole, and not by name.
				p.spineFailed = true
			} else {
				s.refused = append(s.refused, a.url)
			}
			continue
		}
		pulled++
	}

	if pulled > 0 {
		// The BPT, then the commit. Batch.Commit commits the BPT store and
		// never calls Account.putBpt, so a perfectly pulled account leaves the
		// local root exactly where it was and the tracker can never match it
		// (#4305).
		err := p.batch.UpdateBPT()
		if err == nil {
			err = p.batch.Commit()
		} else {
			p.batch.Discard()
		}
		if err != nil {
			s.log.Info("What was pulled could not be written", "partition", s.partition, "error", err)
			s.refused = append(s.refused, p.urls()...)
			return
		}
	} else {
		p.batch.Discard()
	}

	if p.spine {
		s.spineSettled(p)
	}
	s.log.Info("Pulled the accounts the block ledger named", "partition", s.partition,
		"asked", len(p.accounts), "pulled", pulled, "refused", len(p.accounts)-pulled, "root", fmt.Sprintf("%x", root[:4]))
	s.observe(root)
}

// observe tells the tracker the block this node's state now is, when its
// local root is the one the pass proved. The block is read from the ledger
// account in that state: the state hashes into the proven root, so its ledger
// is that root's ledger, where a block number in a peer's answer would be the
// peer's word (#4361, F1).
func (s *PulledState) observe(proven [32]byte) {
	batch := s.db.Begin(false)
	defer batch.Discard()
	local, err := batch.GetBptRootHash()
	if err != nil || local != proven {
		return
	}
	var ledger *protocol.SystemLedger
	if err := batch.Account(s.partition.JoinPath(protocol.Ledger)).Main().GetAs(&ledger); err != nil {
		s.log.Info("The pulled ledger could not be read", "partition", s.partition, "error", err)
		return
	}
	s.tracker.Observe(s.partition, ledger.Index, local)
	if ledger.Index > s.synced {
		s.synced = ledger.Index
	}
}

// dropPass throws the held pass away and asks for all of it again.
func (s *PulledState) dropPass() {
	p := s.pass
	s.pass = nil
	for _, a := range p.accounts {
		a.pending.Discard()
	}
	p.batch.Discard()
	s.refused = append(s.refused, p.urls()...)
}

// oneRoot keeps the accounts that end at the root most of the pass ends at,
// and hands every other one to drop. It returns that root and the block the
// peer said it served it at.
func (p *pass) oneRoot(drop func(a *heldAccount, why string)) (root [32]byte, servedAt uint64) {
	count := map[[32]byte]int{}
	block := map[[32]byte]uint64{}
	for _, a := range p.accounts {
		r := a.pending.Root()
		if r == ([32]byte{}) {
			continue
		}
		count[r]++
		if block[r] == 0 {
			block[r] = a.pending.Block
		}
	}
	for r, n := range count {
		// The later root breaks a tie, so which root wins does not depend on
		// the order a map is walked in.
		if n > count[root] || n == count[root] && block[r] > block[root] {
			root = r
		}
	}

	kept := p.accounts[:0]
	for _, a := range p.accounts {
		switch a.pending.Root() {
		case [32]byte{}:
			drop(a, "it was served with no receipt, so there is no root to prove it against")
		case root:
			kept = append(kept, a)
		default:
			drop(a, "it was served at another root than most of its pass")
		}
	}
	for i := len(kept); i < len(p.accounts); i++ {
		p.accounts[i] = nil
	}
	p.accounts = kept
	return root, block[root]
}

func (p *pass) urls() []*url.URL {
	var out []*url.URL
	for _, a := range p.accounts {
		if !a.spine {
			out = append(out, a.url)
		}
	}
	return out
}

// spineSettled records what became of the spine, and moves the validator sets
// if it verified.
//
// **This is how a joining node crosses a change to the validator sets**, and
// on this line it is the only way. <partition>/network and /globals are
// spine accounts, so they arrive with a receipt that ends at a root proven by
// a quorum of this partition's validators and passes through the leaf the
// pulled body hashes to; adopting them from the store afterwards is
// therefore an induction step and not a peer's word. A change never travels
// in an anchor past Vandenberg (#4301, review finding 1).
func (s *PulledState) spineSettled(p *pass) {
	if p.spineFailed {
		s.log.Info("The spine did not verify; it is asked for again", "partition", s.partition)
		return
	}
	s.spine = true
	s.refreshAuthority()
	s.log.Info("Pulled the spine, verified against a proven root", "partition", s.partition)
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

// fetchPass fetches one pass: the spine until it has verified, then the
// accounts named, up to passLimit. It settles nothing.
//
// The spine is THIS PARTITION'S — its anchors, ledger and operators, with
// their chains — so the chains a join reads are there at all.
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
func (s *PulledState) fetchPass(ctx context.Context, accounts []*url.URL) {
	p := &pass{batch: s.db.Begin(true), since: time.Now(), spine: !s.spine}

	spine := map[string]bool{}
	if p.spine {
		for _, u := range pull.SpineAccounts(s.partition) {
			spine[strings.ToLower(u.String())] = true
			s.fetchOne(ctx, p, u, true)
		}
	}
	for i, u := range accounts {
		if ctx.Err() != nil {
			break
		}
		if len(p.accounts) >= passLimit {
			s.refused = append(s.refused, accounts[i:]...)
			break
		}
		if spine[strings.ToLower(u.String())] {
			continue
		}
		s.fetchOne(ctx, p, u, false)
	}

	if len(p.accounts) == 0 {
		p.batch.Discard()
		if p.spine && !p.spineFailed {
			// Everything the peers have for the spine, this node already has.
			s.spine = true
		}
		return
	}
	s.pass = p
}

// fetchOne fetches one account into the pass, or records why it could not.
func (s *PulledState) fetchOne(ctx context.Context, p *pass, u *url.URL, spine bool) {
	fail := func() {
		if spine {
			p.spineFailed = true
		} else {
			s.refused = append(s.refused, u)
		}
	}
	srcs, partition, err := s.sourcesFor(ctx, u)
	switch {
	case errors.Is(err, errNotThisPartition):
		// Dropped, not refused: a peer named an account this store must not
		// hold, and asking again will not change that.
		s.log.Info("A named account is not this partition's and was dropped",
			"account", u, "partition", s.partition)
		return
	case err != nil:
		s.log.Info("No peer could be found for an account", "account", u, "error", err)
		fail()
		return
	}
	mode := pull.ModeStateOnly
	if spine {
		mode = pull.ModeFullSpine
	}
	pending, _, err := pull.FetchFrom(ctx, srcs, p.batch, u, pull.Options{
		Mode:      mode,
		Verify:    s.anchors,
		Partition: partition,
	})
	if err != nil {
		s.log.Info("An account could not be pulled", "account", u, "error", err)
		fail()
		return
	}
	p.accounts = append(p.accounts, &heldAccount{url: u, pending: pending, spine: spine})
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
//
// It is the local root's block every time it is asked, not the block of the
// first match. The machine goes ACTIVE once, at the first match, and a join
// that found a gap after it pulls on, so the state moves past the block the
// machine names; answering with that block would settle staging against a
// state it is not (#4362).
func (s *PulledState) Matched(ctx context.Context) (uint64, bool, error) {
	ok, err := s.tracker.Check(ctx)
	if err != nil {
		return 0, false, errors.UnknownError.Wrap(err)
	}
	if !ok && s.machine.State() != nodestate.StateActive {
		return 0, false, nil
	}

	batch := s.db.Begin(false)
	local, err := batch.GetBptRootHash()
	batch.Discard()
	if err != nil {
		return 0, false, errors.UnknownError.WithFormat("read the local root: %w", err)
	}
	for _, o := range s.tracker.Snapshot() {
		if o.Anchor == local {
			return o.Block, true, nil
		}
	}
	return 0, false, nil
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
