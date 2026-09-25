// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/enumerate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/tracker"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	bpt "gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
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

// walkPagesPerRound is how many pages of the peer's BPT one round walks. The
// walk and the block-ledger records share a round, the records first, so a
// round is bounded by what the walk takes of it and the records keep up with
// the partition however large its tree is (executor spec, "Sync", "The
// algorithm", steps 1 and 2).
const walkPagesPerRound = 8

// walkPageSize is how many leaves one page asks for.
const walkPageSize = 256

// PulledState is the join's state half (#4293). It follows the algorithm of
// the executor spec, "Sync", "The algorithm", steps 1-3:
//
//  1. The pull starts at the peer's block S. It walks the peer's whole BPT,
//     page by page, and pulls every account whose leaf this node does not
//     hold or holds differently; alongside it, it takes the block-ledger
//     record of every block after S, in block order, and pulls every account
//     each record names. The walk never overwrites a value the records
//     wrote: an account a record has brought current is skipped when the walk
//     reaches it, because the walk's page may be older than that record.
//  2. Until the match, every new block's record is processed as it comes.
//  3. The match is the proof: the local root equal to the StateTreeAnchor of
//     an anchor a quorum of this partition's validators signed (tracker,
//     anchorsrc). Nothing before it is proven, and nothing before it needs
//     to be.
//
// Every read is addressed at a NAMED PEER. The node's own routed client
// answers from the node's own store for any service it provides, which for a
// joining node is the un-executed store the pull exists to fill (#4303).
type PulledState struct {
	partition *url.URL
	db        *database.Database
	sources   Sources

	// anchors is where this partition's verified roots come from: its own
	// anchors, collected from its validators (anchorsrc.Collector).
	anchors anchorSource

	// authority is the validator sets this node trusts. It is seeded from
	// this node's OWN store before anything is pulled, and it moves only
	// when the state has matched: <partition>/network and /globals are
	// pulled like every account, and what the pull writes is proven by the
	// match and by nothing before it (#4301, #4438).
	authority *anchorsrc.Authority

	machine *nodestate.Machine
	tracker *tracker.Tracker
	log     *slog.Logger

	// label is the partition as the metrics name it.
	label string

	// now is the clock the anchor source's lines are paced by; nil is
	// time.Now.
	now func() time.Time

	// What was last said about the anchor source, so that a stall is said
	// once a minute and not once per peer per round (#4419).
	sayMu    sync.Mutex
	stallLog struct {
		on    bool
		entry uint64
		since time.Time
		at    time.Time
	}
	refusalLog struct {
		key string
		at  time.Time
	}

	// matched is the last match Matched reported: the block the local root
	// was anchored for and the root. Promote reads it, because by the time
	// the handoff has succeeded the node has produced blocks after it and
	// the local root is no longer the root that matched.
	matched tracker.Match

	// executed is the block this node's EXECUTOR last executed. It is read
	// once, before anything is pulled, and never from the store again: the
	// ledger account is one of the accounts the pull overwrites (#4295,
	// #4344). After the handoff it is the last block whose root matched its
	// anchor (HandedOff, Diverged).
	executed uint64

	// sync is the pull in progress: nil until the first round starts one, and
	// again after the handoff, so that a node that syncs again starts a new
	// pull from the peer's block then.
	sync *syncing

	// provenAt is the last block whose root matched its signed anchor.
	provenAt uint64

	// repairFrom, when not zero, is where the next pull starts as a repair
	// (RepairFrom; startRepair).
	repairFrom uint64

	// headOnly are the accounts taken by their chain heads alone, whose
	// entries below the open mark set the node does not hold yet. Once the
	// state has matched they are backfilled while the node executes
	// (backfill).
	headOnly map[[32]byte]*url.URL

	// entire are the accounts this node holds whole, every entry of every
	// chain with the message behind it (backfillOne). A whole pull of one
	// goes on from what it holds, so it stays entire.
	entire map[[32]byte]bool

	// pageTurn and sourceTurn rotate which peer a walk page and an account
	// are asked of first, so no one peer answers every time (#4438 threat
	// F6).
	pageTurn   int
	sourceTurn int

	// served names the peers whose answers the last pull handed off from
	// took, and demoted counts, per peer, the pulls it served that did not
	// bring the match (order).
	served  map[string]bool
	demoted map[string]int

	// repairs counts the repairs since the last match. A repair from the
	// block ledger that did not bring a match is followed by one that walks
	// the tree again (startRepair).
	repairs int

	// checkedHeld is whether this process has checked the entries the node
	// already holds on the accounts it takes whole for their messages
	// (pull.Options.CheckHeld). Once a process.
	checkedHeld bool
}

// syncing is one pull: the two cursors of the algorithm, and what the records
// have brought current.
type syncing struct {
	// start is S, the peer's block when the pull started.
	start uint64

	// last is L, the last block whose block-ledger record has been
	// processed. It starts at S.
	last uint64

	// cursor is where the next page of the walk starts, and walked says the
	// walk has covered the whole tree.
	cursor [32]byte
	walked bool
	pages  int

	// current is every account a processed record named. The walk skips
	// them: its page may be older than the record (step 1).
	current map[[32]byte]bool

	// retry is what a pull could not take and must take again: an account
	// a record named, or a leaf the walk found stale, that no peer served
	// this time. It is asked for again every round until it is taken or
	// every peer says it has no leaf for it.
	retry map[[32]byte]*url.URL

	// spine is whether the partition's spine has been taken whole once.
	spine bool

	// repair is whether this pull repairs what the node executed
	// (startRepair).
	repair bool

	// low is the oldest block the records have been read from: S, or the
	// block of a walk page served before it (walk). want is the oldest block
	// they must be read from: a walk page served before low moves want, and
	// the records are read back to it at most maxRecordsPerRound blocks a
	// round (readBack).
	low  uint64
	want uint64

	// named are the leaves the walk's pages have named, by key hash; a leaf
	// the node holds that none named is dropped when the walk completes.
	named map[[32]byte]bool

	// servedBy names the peers whose answers this pull took.
	servedBy map[string]bool

	// ready is whether the last round left the walk done and nothing owed:
	// the state stands at last, and the node may execute from there (Ready).
	ready bool
}

func newSyncing() *syncing {
	return &syncing{current: map[[32]byte]bool{}, retry: map[[32]byte]*url.URL{}, named: map[[32]byte]bool{}, servedBy: map[string]bool{}}
}

func accountKey(u *url.URL) [32]byte { return u.AccountID32() }

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
	track, err := tracker.New(opts.Database, opts.Partition)
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
	// Recorded under SystemData, where no pull reaches, and not read from
	// <partition>/network, an account the pull overwrites before anything is
	// proven: a node that restarted between a pull and its match would
	// otherwise trust a peer's validators (#4438 F1).
	authority, err := anchorsrc.FromTrusted(opts.Database, opts.Partition)
	if err != nil {
		// A node that cannot say who the validators are cannot verify a root,
		// and a join that cannot verify a root is the defect this closes. It
		// does not proceed unverified.
		return nil, errors.UnknownError.WithFormat(
			"this node cannot say who %v's validators are, so it cannot verify anything it pulls: %w",
			opts.Partition, err)
	}

	// The partition's OWN anchors, collected from its validators: each
	// signs its own copy of the anchor of block B as B closes, and the anchor
	// is taken when distinct members reaching the partition's threshold have
	// signed it (executor spec, "Sync", "The algorithm", step 3). Not the
	// Directory's copy: that reaches the Directory's pool only after the
	// Directory executes it, far too late to match a partition that moves
	// every block (Paul, 2026-09-25). The Directory's own anchors are
	// collected from the Directory's validators the same way. Never from this
	// node (#4303).
	s.authority = authority
	sources := opts.Sources
	pool := opts.Partition.JoinPath(protocol.AnchorPool)
	collector, err := anchorsrc.NewCollector(opts.Partition, authority, sources, func(ctx context.Context) ([]anchorsrc.AccountReader, error) {
		srcs, _, err := sources.For(ctx, pool)
		if err != nil {
			return nil, err
		}
		out := make([]anchorsrc.AccountReader, len(srcs))
		for i, src := range srcs {
			out[i] = src
		}
		return out, nil
	})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	collector.OnAnchor = track.Observe
	collector.OnRefused = s.anchorRefused
	s.anchors = collector

	// Watchable from the moment the node starts joining, and on every change.
	// The daemon has already reported this partition's state — the series
	// exists whether or not a node joins (#4345a) — so this is an update of
	// something already on the wire, not its creation.
	label := opts.Partition.String()
	if id, ok := protocol.ParsePartitionUrl(opts.Partition); ok {
		label = id
	}
	s.label = label
	mSpineStalled.WithLabelValues(label).Set(-1)
	nodestate.Report(label, machine.State())
	machine.OnChange(func(ad nodestate.Advertisement) {
		nodestate.Report(label, ad.State)
	})
	return s, nil
}

// anchorSource is what the join reads its partition's verified roots from.
type anchorSource interface {
	Read(ctx context.Context) error
	Stalled() (anchorsrc.Stall, bool)
	Rewind()
}

// mSpineStalled is the sequence number of the anchor this node's anchor
// source is held at, or -1: an anchor a validator produced and no quorum of
// the partition's validators signed. A join held there reads no root after it
// and so matches nothing after it, while every other sign of the join says
// only BOOTING (#4419).
var mSpineStalled = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "accumulate",
	Subsystem: "join",
	Name:      "spine_stalled_entry",
	Help: "The sequence number of the partition's own anchor the join's anchor source is held at " +
		"because no quorum of the partition's validators signed it; -1 when it is not held",
}, []string{"partition"})

// stallSayEvery is how often a stall, and a refusal repeated word for word,
// is said again.
const stallSayEvery = time.Minute

func (s *PulledState) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// readAnchors reads what the anchor pool has gained and says whether the read
// is held at an entry: on the gauge every round, in the log once a minute and
// once when it moves on (#4419).
func (s *PulledState) readAnchors(ctx context.Context) error {
	err := s.anchors.Read(ctx)

	st, held := s.anchors.Stalled()
	s.sayMu.Lock()
	defer s.sayMu.Unlock()
	now := s.clock()
	if !held {
		mSpineStalled.WithLabelValues(s.label).Set(-1)
		if s.stallLog.on {
			s.log.Info("The anchor source moved past the anchor it was held at",
				"partition", s.partition, "anchor", s.stallLog.entry, "after", now.Sub(s.stallLog.since).Round(time.Second))
			s.stallLog.on = false
		}
		return err
	}

	mSpineStalled.WithLabelValues(s.label).Set(float64(st.Entry))
	if !s.stallLog.on || s.stallLog.entry != st.Entry {
		s.stallLog.on, s.stallLog.entry, s.stallLog.since = true, st.Entry, now
	} else if now.Sub(s.stallLog.at) < stallSayEvery {
		return err
	}
	s.stallLog.at = now
	s.log.Info("This partition's spine is stalled: no quorum of its validators signed this anchor, so no root after it is read",
		"partition", s.partition, "anchor", st.Entry,
		"for", now.Sub(s.stallLog.since).Round(time.Second), "asked", st.Asked, "error", st.Err)
	return err
}

func (s *PulledState) spineStalled() bool {
	s.sayMu.Lock()
	defer s.sayMu.Unlock()
	return s.stallLog.on
}

// anchorRefused says an anchor was refused. Said out loud: a source that
// records nothing looks exactly like a network that has anchored nothing,
// and the difference between them is the difference between a peer lying and
// a peer being slow. Said once a minute when it is the same refusal: a held
// entry is asked of every peer every round, and each says the same (#4419).
func (s *PulledState) anchorRefused(block uint64, err error) {
	s.sayMu.Lock()
	defer s.sayMu.Unlock()
	key := fmt.Sprint(block, err)
	now := s.clock()
	if key == s.refusalLog.key && now.Sub(s.refusalLog.at) < stallSayEvery {
		return
	}
	s.refusalLog.key, s.refusalLog.at = key, now
	if block == 0 {
		// An entry served without its body names no block; the error names
		// the entry (#4418).
		s.log.Info("An anchor was refused", "partition", s.partition, "error", err)
		return
	}
	s.log.Info("An anchor was refused", "partition", s.partition, "block", block, "error", err)
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

// Pull runs one round of the algorithm (executor spec, "Sync", "The
// algorithm", steps 1 and 2): the block-ledger records of every block after the
// last one processed, in block order, then the next pages of the walk, then
// what an earlier round could not take. Everything it pulls it writes; the
// match is the one proof (Matched).
func (s *PulledState) Pull(ctx context.Context) error {
	// The anchors first: they are what the match is judged against.
	//
	// A failure here ends the round, never the join. Every read is addressed
	// at a named peer, so any of them can fail for the ordinary reason that
	// the peer it picked is restarting -- and under chaos that is a
	// certainty. Returning an error reaches join.Run, which returns, and the
	// daemon then abandons the join for good.
	err := s.readAnchors(ctx)
	if err != nil && !(errors.Is(err, errors.NotReady) && s.spineStalled()) {
		// A read held at an entry every peer refuses is the stall, said
		// once a minute by readAnchors, not once a round here.
		s.log.Info("This partition's anchors could not be read this round",
			"partition", s.partition, "error", err)
	}

	q := api.Querier2{Querier: s.sources.Querier(s.partition)}
	peer, err := s.peersBlock(ctx)
	if err != nil {
		// No peer could say where the partition is. Nothing is lost by
		// waiting: the records are read from where they stopped.
		s.log.Info("No peer could say which block the partition is at this round", "partition", s.partition, "error", err)
		return nil
	}

	if s.sync == nil && s.repairFrom != 0 {
		s.startRepair(ctx)
	}
	if s.sync == nil {
		s.sync = newSyncing()
		s.sync.start, s.sync.last, s.sync.low, s.sync.want = peer, peer, peer, peer
		s.log.Info("Pulling the state: the whole BPT, and every block-ledger record from here on",
			"partition", s.partition, "start", peer, "executed", s.executed)
	}
	p := s.sync
	p.ready = false

	// The spine, whole, once: this partition's anchors, ledger, operators and
	// network definition, with their chains and the messages behind them
	// (§1, §3). It is pulled like the walk -- a record processed later pulls
	// again whatever of it a later block changed.
	if !p.spine {
		ok := true
		for _, u := range pull.SpineAccounts(s.partition) {
			if p.current[accountKey(u)] {
				continue
			}
			if s.pullOne(ctx, p, u) != taken {
				ok = false
			}
		}
		p.spine = ok
		if ok {
			s.checkedHeld = true
		}
	}

	// Step 1, the records: every block after L through the peer's block, in
	// block order, and every account each names pulled and written. Every
	// account is marked current as its record names it, whether or not the
	// pull takes it this round, so the walk never writes over it.
	s.processRecords(ctx, q, p, peer)

	// What an earlier round could not take, once each this round.
	owed := p.retry
	p.retry = map[[32]byte]*url.URL{}
	for _, u := range owed {
		if ctx.Err() != nil {
			p.retry[accountKey(u)] = u
			continue
		}
		s.pullOne(ctx, p, u)
	}

	// Step 1, the walk: the next pages of the peer's BPT.
	if !p.walked {
		s.walk(ctx, q, p)
	}
	s.readBack(ctx, q, p)

	// With the walk done, nothing owed and the records read back to the
	// oldest page's block, the state is the partition's at L if the pull is
	// right. The node may execute from here, comparing at every block that
	// anchors (Ready; DIFFERENCES.md E11: the spec's "Two mismatches"
	// executes only once the root has matched).
	p.ready = p.walked && len(p.retry) == 0 && p.low <= p.want
	return nil
}

// Ready reports the block the pulled state stands at once the walk has covered
// the tree and nothing is owed a retry: the block the node may hand off at and
// execute from, unproven, comparing its root with the partition's signed
// anchor at every block that sends one. A match there is the proof, and
// Diverged promotes the node at it; a mismatch is repaired from the block
// ledger. The spec's "Two mismatches" executes nothing until the root has
// matched; handing off unproven is today's departure from it (DIFFERENCES.md
// E11), kept until the as-of-block pulls land (#4442).
//
// The block is the one the pulled ledger names, not the last record read: the
// ledger is pulled as it is on the peer now, and the peer may have moved past
// the last record while the round pulled. A block the records have not reached
// is wrong only in the accounts its record names, which the repair brings
// current if the root does not match.
func (s *PulledState) Ready() (uint64, bool) {
	if s.sync == nil || !s.sync.ready {
		return 0, false
	}
	n, err := readExecutedBlock(s.db, s.partition)
	if err != nil || n < s.sync.last {
		return 0, false
	}
	return n, true
}

// startRepair begins the pull that repairs the state after an executed
// block's root differed from its signed anchor (executor spec, "Sync",
// "Two mismatches"). There is no walk: for every block
// since the last match -- or since the pull began, if the state has never
// matched -- it pulls again every account the partition's record names (the
// records, from repairFrom on) and every account this node's own record names
// (read here, from its own block ledger, through the block its executor last
// executed). An account no peer holds a leaf for is deleted (pullOne): a node
// that executed an account into existence that no peer holds loses it here,
// because its own record names it.
func (s *PulledState) startRepair(ctx context.Context) {
	from := s.repairFrom
	s.repairFrom = 0
	p := newSyncing()
	p.start, p.last, p.low, p.want = from, from, from, from
	p.walked, p.spine, p.repair = true, true, true
	s.sync = p

	// A repair from the block ledger that did not bring a match is followed
	// by one that pulls the accounts again: the walk, against the peers,
	// taking whole every account whose leaf differs and dropping the leaves
	// no peer holds (Paul, 2026-09-25: "If the pull of accounts doesn't
	// create a matching BPT root, then the joining node has to pull the
	// accounts again. I don't see what else it can do."). An account the
	// walk took wrongly, that no later block names, is beyond any record.
	s.repairs++
	if s.repairs > 1 {
		p.walked = false
		s.log.Info("A repair from the block ledger did not bring a match; walking the tree again",
			"partition", s.partition, "repairs", s.repairs)
	}

	var executed uint64
	if id, ok := protocol.ParsePartitionUrl(s.partition); ok {
		n, err := LastExecutedBlock(s.db, id)
		if err != nil {
			s.log.Info("This node's executed block could not be read for the repair", "partition", s.partition, "error", err)
		}
		executed = n
	}
	var own []*url.URL
	batch := s.db.Begin(false)
	ledger := batch.Account(s.partition.JoinPath(protocol.Ledger))
	for n := from + 1; n <= executed; n++ {
		_, entries, err := indexing.LoadBlockLedger(ledger, n)
		switch {
		case err == nil:
			own = append(own, ChangedAccounts(s.partition, entries)...)
		case errors.Is(err, errors.NotFound):
			// A block this node executed empty, or did not execute.
		default:
			s.log.Info("This node's own block-ledger record could not be read", "partition", s.partition, "block", n, "error", err)
		}
	}
	batch.Discard()

	s.log.Info("Repairing the state from the block ledger: every account named since the last match, by the partition's records and this node's own",
		"partition", s.partition, "from", from, "executed", executed, "own", len(own))
	seen := map[[32]byte]bool{}
	for _, u := range own {
		k := accountKey(u)
		if seen[k] || ctx.Err() != nil {
			continue
		}
		seen[k] = true
		p.current[k] = true
		s.pullOne(ctx, p, u)
	}
}

// maxRecordsPerRound bounds how many blocks of records one round reads, so a
// target set by a peer cannot make one round a query per block without end
// (#4438 threat F3). The next round goes on from where this one stopped.
const maxRecordsPerRound = 1024

// processRecords processes the block-ledger records of the blocks after L
// through through, in block order, and moves L to the last one read. A record
// that cannot be read stops it there, and the next round goes on from it.
func (s *PulledState) processRecords(ctx context.Context, q api.Querier2, p *syncing, through uint64) {
	if through > p.last+maxRecordsPerRound {
		through = p.last + maxRecordsPerRound
	}
	last := s.processRange(ctx, q, p, p.last, through)
	if last > p.last {
		p.last = last
	}
}

// recordWindow is how many blocks back from the newest verified anchor the
// records are read: the peers' retention (BPTHistoryDepth, 1024 blocks by
// default; executor spec, "Sync").
const recordWindow = 1024

// recordFloor is the oldest block the records are ever read back to: the
// newest block a quorum of the partition's validators signed an anchor for,
// less the peers' retention window.
func (s *PulledState) recordFloor() uint64 {
	n := s.tracker.LatestObservedBlock()
	if n <= recordWindow {
		return 0
	}
	return n - recordWindow
}

// readBack reads the records from low back towards want, at most
// maxRecordsPerRound blocks a round, as the forward direction is bounded
// (processRecords). A record that cannot be read leaves low where it was, and
// the next round reads the same span again.
func (s *PulledState) readBack(ctx context.Context, q api.Querier2, p *syncing) {
	if p.want >= p.low || ctx.Err() != nil {
		return
	}
	from := p.want
	if p.low-from > maxRecordsPerRound {
		from = p.low - maxRecordsPerRound
	}
	if s.processRange(ctx, q, p, from, p.low) == p.low {
		p.low = from
	}
}

// processRange reads the records of the blocks after after through through,
// in block order, marks every account they name current and pulls it, and
// returns the last block read. Every pull is of the peer's state now, which is
// at or after every block read, so an account two blocks named is current
// after one pull.
func (s *PulledState) processRange(ctx context.Context, q api.Querier2, p *syncing, after, through uint64) uint64 {
	var named []*url.URL
	last := after
	for n := after + 1; n <= through; n++ {
		entries, err := blockLedgerOf(ctx, q, s.partition, n)
		switch {
		case err == nil:
		case errors.Is(err, errors.NotFound):
			// An empty block writes nothing, not even its index: it changed
			// nothing.
		default:
			s.log.Info("The block ledger could not be read; the records go on from here next round",
				"partition", s.partition, "block", n, "error", err)
			through = n - 1
		}
		if n > through {
			break
		}
		for _, u := range ChangedAccounts(s.partition, entries) {
			p.current[accountKey(u)] = true
			named = append(named, u)
		}
		last = n
	}

	seen := map[[32]byte]bool{}
	for _, u := range named {
		k := accountKey(u)
		if seen[k] || ctx.Err() != nil {
			continue
		}
		seen[k] = true
		s.pullOne(ctx, p, u)
	}
	return last
}

// walk takes the next pages of the peer's BPT and pulls every account whose
// leaf this node does not hold, or holds and does not agree with, unless a
// record has already brought it current: the page may be older than that
// record, and the walk never overwrites what a record wrote (step 1). A page
// newer than the records needs no rule: the account it wrote was changed by a
// block the records have not reached, and that block's record pulls it again.
//
// A page OLDER than the records' start is the other hazard: an account
// changed at a block after the page's and at or before S shows its old leaf,
// which the node may hold too, so the walk skips it, and no record after S
// names it. Each page is read from one named peer whose block is read first,
// and the records are processed from the oldest block a page was served at,
// not from S (Paul, 2026-09-25; #4438 review F1).
//
// Once the walk has covered the tree, every leaf the node holds that no page
// named, and that no record brought current, is an account no peer holds: it
// is deleted (Paul: "It must also drop leaves it holds that no peer holds").
func (s *PulledState) walk(ctx context.Context, q api.Querier2, p *syncing) {
	for i := 0; i < walkPagesPerRound && !p.walked && ctx.Err() == nil; i++ {
		pager, served, ok := s.pageSource(ctx)
		if !ok {
			pager, served = q, p.low
		}
		batch := s.db.Begin(false)
		page, err := enumerate.ReadPage(ctx, pager, s.partition, batch, p.cursor, walkPageSize)
		batch.Discard()
		if err != nil {
			s.log.Info("The peer's BPT could not be paged this round; the walk goes on from here next round",
				"partition", s.partition, "pages", p.pages, "error", err)
			return
		}
		p.pages++
		if served < p.want {
			// A peer's word for where it stands is not a floor the node
			// must read back to: one that answers 0 would make the records
			// a query per block since genesis. Nothing older than the
			// newest verified anchor's block less the peers' retention
			// can be served as of its block anyway (#4438 re-review F-1).
			if floor := s.recordFloor(); served < floor {
				s.log.Info("A walk page's peer stands further back than the peers retain; the records are read back only to the retention floor",
					"partition", s.partition, "page-block", served, "floor", floor)
				served = floor
			}
		}
		if served < p.want {
			s.log.Info("A walk page was served at a block before the records' start; the records are read from there",
				"partition", s.partition, "page-block", served, "records-from", p.low)
			p.want = served
		}

		for _, e := range page.Record.Entries {
			if e != nil {
				p.named[e.KeyHash] = true
			}
		}
		for _, u := range page.Stale {
			if p.current[accountKey(u)] {
				continue
			}
			s.pullOne(ctx, p, u)
		}

		if page.Record.Done {
			p.walked = true
			s.log.Info("The walk has covered the peer's whole BPT", "partition", s.partition,
				"pages", p.pages, "records-through", p.last, "start", p.start, "records-from", p.low)
			s.dropUnnamed(ctx, p)
			return
		}
		p.cursor = page.Record.NextStart
	}
}

// pageSource is the peer the next walk page is read from, named, and the block
// its ledger says it stands at: the page is of that block or a later one. The
// peers are taken in rotation. False when the peers cannot be read that way.
func (s *PulledState) pageSource(ctx context.Context) (enumerate.Source, uint64, bool) {
	srcs, _, err := s.sources.For(ctx, s.partition.JoinPath(protocol.Ledger))
	if err != nil || len(srcs) == 0 {
		return nil, 0, false
	}
	src := srcs[s.pageTurn%len(srcs)]
	s.pageTurn++
	pager, ok := src.(enumerate.Source)
	if !ok {
		return nil, 0, false
	}
	n, err := ledgerIndexOf(ctx, src, s.partition)
	if err != nil {
		return nil, 0, false
	}
	return pager, n, true
}

// dropUnnamed asks the peers for every account the node holds a leaf for that
// no page of the walk named and no record brought current. Nothing is deleted
// on the pages' word, which is a peer's: an account every peer answers
// NotFound for is deleted (pullOne, forget), and one some peer serves is
// taken (executor spec, "Sync", "Two mismatches", 1: "It deletes nothing on
// this comparison").
func (s *PulledState) dropUnnamed(ctx context.Context, p *syncing) {
	var unnamed []*url.URL
	batch := s.db.Begin(false)
	err := batch.ForEachAccount(func(a *database.Account, _ [32]byte) error {
		u := a.Url()
		if p.named[bpt.NewKey("Account", u).Hash()] || p.current[accountKey(u)] {
			return nil
		}
		unnamed = append(unnamed, u)
		return nil
	})
	batch.Discard()
	if err != nil {
		s.log.Info("This node's leaves could not be listed after the walk", "partition", s.partition, "error", err)
		return
	}
	for _, u := range unnamed {
		if ctx.Err() != nil {
			return
		}
		s.pullOne(ctx, p, u)
	}
}

// An outcome is what became of one account's pull.
type outcome int

const (
	taken   outcome = iota // pulled and written
	dropped                // no peer has a leaf for it, or it is not this partition's
	owed                   // not taken this time; asked for again next round
)

// pullOne pulls one account from this partition's peers and writes it, with
// the BPT, in a batch of its own. What it cannot take is owed to the next
// round, whoever named it.
//
// The spine and the partition's synthetic ledger are taken whole, with every
// chain entry and the message behind each (pull.WholeAccounts, #4421, #4434);
// every other account state-only. Nothing is verified against a root here:
// the match is the proof (step 3). What the pull refuses as malformed it still
// refuses -- a body served under another name (#4408), an answer with no body
// (#4437), an entry with no message behind it (#4400) -- and the next peer is
// asked.
func (s *PulledState) pullOne(ctx context.Context, p *syncing, u *url.URL) outcome {
	if !Routable(u) {
		return dropped
	}
	srcs, partition, err := s.sourcesFor(ctx, u)
	switch {
	case errors.Is(err, errNotThisPartition):
		// Dropped, not owed: a peer named an account this store must not
		// hold, and asking again will not change that.
		s.log.Info("A named account is not this partition's and was dropped",
			"account", u, "partition", s.partition)
		return dropped
	case err != nil:
		s.log.Info("No peer could be found for an account", "account", u, "error", err)
		p.retry[accountKey(u)] = u
		return owed
	}

	// A repair takes every account whole: main state, every chain with its
	// entries and the messages behind them, pending and directory (executor
	// spec, "Sync", "Two mismatches").
	srcs = s.order(srcs)

	whole := s.takenWhole(u) || p.repair
	mode := pull.ModeStateOnly
	if whole {
		mode = pull.ModeFullSpine
	}
	batch := s.db.Begin(true)
	defer batch.Discard()
	pending, served, err := pull.FetchFrom(ctx, srcs, batch, u, pull.Options{
		Mode:      mode,
		Partition: partition,
		// The answer with a receipt is the one that carries the rest of the
		// leaf beside the body (#4399), and the one whose NotFound says the
		// peer holds no leaf (#4397).
		WithReceipt: true,
		// Once a process, every account taken whole is checked for what an
		// earlier join left without its message -- the synthetic ledger
		// included, which every join before #4434 took state-only.
		CheckHeld: whole && !s.checkedHeld,
		// A node that has executed since its last match repairs what it
		// executed: a chain of its own longer than the peer's is taken again.
		RetakeLonger: p.repair,
	})
	if err == nil && served >= 0 && served < len(srcs) {
		p.servedBy[sourceKey(srcs[served])] = true
	}
	defer func() {
		// What was taken by its heads alone is backfilled once the state
		// has matched (backfill). A whole pull goes on from the entries the
		// node holds, so an account taken by its heads before stays to be
		// backfilled below them.
		if err != nil || whole && !p.repair {
			return
		}
		if p.repair {
			// The repair takes the account whole, entries included, now,
			// while the node is not executing. An account already held
			// entire needs only what the whole pull appended.
			if s.entire[accountKey(u)] || s.backfillOne(ctx, u) {
				delete(s.headOnly, accountKey(u))
				return
			}
		}
		if s.headOnly == nil {
			s.headOnly = map[[32]byte]*url.URL{}
		}
		s.headOnly[accountKey(u)] = u
	}()
	switch {
	case err == nil:
	case stderrors.Is(err, pull.ErrNoLeaf):
		// Dropped, not owed (#4397): every source was asked and every one
		// answered that its tree holds no leaf for the name. Asking again
		// changes nothing, and a record names it again if a block ever
		// gives it one.
		s.forget(u)
		return dropped
	default:
		s.log.Info("An account could not be pulled; it is asked for again next round", "account", u, "error", err)
		p.retry[accountKey(u)] = u
		return owed
	}

	// The BPT, then the commit. Batch.Commit commits the BPT store and never
	// calls Account.putBpt, so a perfectly pulled account leaves the local
	// root exactly where it was and the match can never come (#4305).
	err = pending.Keep()
	if err == nil {
		err = batch.UpdateBPT()
	}
	if err == nil {
		err = batch.Commit()
	}
	if err != nil {
		s.log.Info("What was pulled could not be written; it is asked for again next round",
			"account", u, "partition", s.partition, "error", err)
		p.retry[accountKey(u)] = u
		return owed
	}
	return taken
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

// peersBlock is the block the partition's peers stand at: the LOWEST block any
// of them names. One peer's word is not a target the node must reach -- a
// peer that named a block far ahead would set S there and hold the records
// off, or make a round a query per block without end (#4438 threat F3).
//
// The lowest is floored at the newest block a quorum of the partition's
// validators signed an anchor for, as this node has verified it: the
// partition has certainly reached that block, so a peer that names a block
// before it is behind -- a laggard, or one answering 0 -- and is not counted
// (#4438 re-review F-2). Otherwise one such peer would pin the target, and
// with it L, where it stands.
func (s *PulledState) peersBlock(ctx context.Context) (uint64, error) {
	srcs, _, err := s.sources.For(ctx, s.partition.JoinPath(protocol.Ledger))
	if err != nil {
		return 0, errors.UnknownError.WithFormat("find the partition's peers: %w", err)
	}
	floor := s.tracker.LatestObservedBlock()
	var low uint64
	var found bool
	var last error
	for _, src := range srcs {
		n, err := ledgerIndexOf(ctx, src, s.partition)
		if err != nil {
			last = err
			continue
		}
		if n < floor {
			last = errors.NotReady.WithFormat("%v stands at block %d, before the newest signed anchor's block %d", src, n, floor)
			continue
		}
		if !found || n < low {
			low, found = n, true
		}
	}
	if !found {
		return 0, errors.UnknownError.WithFormat("no peer served its ledger: %w", last)
	}
	return low, nil
}

// ledgerIndexOf is the block one peer's ledger names.
func ledgerIndexOf(ctx context.Context, src pull.Source, partition *url.URL) (uint64, error) {
	rec, err := src.QueryAccount(ctx, partition.JoinPath(protocol.Ledger), nil)
	if err != nil {
		return 0, errors.UnknownError.WithFormat("read a peer's ledger: %w", err)
	}
	ledger, ok := rec.Account.(*protocol.SystemLedger)
	if !ok {
		return 0, errors.Conflict.WithFormat("a peer served %v as %v, not a system ledger",
			partition.JoinPath(protocol.Ledger), rec.Account.Type())
	}
	return ledger.Index, nil
}

// TrustedVersion is the network definition version this join verifies
// anchors against. It moves only when refreshAuthority takes a definition
// out of a state that matched.
func (s *PulledState) TrustedVersion() uint64 { return s.authority.Version() }

// refreshAuthority takes the validator sets out of this node's store.
//
// **It is called only when the store is proven**, which is what makes this an
// induction step and not a peer's word: at the start, when the store is the
// node's own execution, and when the local root has just matched a root a
// quorum of the trusted set signed (Matched). Between the two the store holds
// what the pull wrote, which nothing has proven yet (executor spec, "Sync",
// "The algorithm", step 3): a network definition read from it then would be
// whatever a peer served, and anchors verified against it would be that
// peer's too (#4301).
//
// So a change to the validator sets during a join is crossed at the match and
// not before it: until then anchors are judged by the set the node started
// with, to its threshold, with the anchor's declared version a floor (§1). A
// change that turns over more of the set than the old threshold can bridge
// holds the match off, as it did before (§1's stated limit).
func (s *PulledState) refreshAuthority() {
	moved, err := s.authority.UpdateFrom(s.db, s.partition)
	if err == nil {
		// The state is proven: what the sets are now is what a restart
		// trusts (FromTrusted).
		err = s.authority.Remember(s.db, s.partition)
	}
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

// order is the order the peers are asked in for one account: in rotation, so
// a peer that serves a wrong body is not the one asked first every time
// (#4438 threat F6), and with a peer whose answers were part of a pull that
// did not bring the match moved to the back, the more such pulls the further
// back (executor spec, "Sync", "Two mismatches", 1).
func (s *PulledState) order(srcs []pull.Source) []pull.Source {
	n := len(srcs)
	if n < 2 {
		return srcs
	}
	turn := s.sourceTurn % n
	s.sourceTurn++
	out := append(append([]pull.Source(nil), srcs[turn:]...), srcs[:turn]...)
	sort.SliceStable(out, func(i, j int) bool {
		return s.demoted[sourceKey(out[i])] < s.demoted[sourceKey(out[j])]
	})
	return out
}

// sourceKey names a source: the peer it reaches, where it says.
func sourceKey(src pull.Source) string {
	if s, ok := src.(fmt.Stringer); ok {
		return s.String()
	}
	return fmt.Sprintf("%T", src)
}

// forget deletes an account no peer holds a leaf for, if this node holds it
// (executor spec, "Sync", "Two mismatches": "deleting any
// the peers do not hold"). A node that executed an account into existence, or
// that holds one from before a block that removed it, would otherwise keep a
// leaf no peer has, and its root could never match.
func (s *PulledState) forget(u *url.URL) {
	batch := s.db.Begin(false)
	defer batch.Discard()
	_, err := batch.BPT().Get(bpt.NewKey("Account", u))
	switch {
	case errors.Is(err, errors.NotFound):
		s.log.Info("No peer holds a leaf for a named account; it was dropped",
			"account", u, "partition", s.partition)
		return
	case err != nil:
		s.log.Info("This node's leaf for an account could not be read", "account", u, "error", err)
		return
	}
	batch2 := s.db.Begin(true)
	defer batch2.Discard()
	err = batch2.ForgetAccount(u)
	if err == nil {
		err = batch2.Commit()
	}
	if err != nil {
		s.log.Info("An account no peer holds could not be deleted", "account", u, "error", err)
		return
	}
	s.log.Info("No peer holds a leaf for an account this node holds; it was deleted",
		"account", u, "partition", s.partition)
}

// takenWhole is whether u is one of the accounts this partition's join takes
// whole whenever it pulls it (pull.WholeAccounts).
func (s *PulledState) takenWhole(u *url.URL) bool {
	for _, a := range pull.WholeAccounts(s.partition) {
		if a.Equal(u) {
			return true
		}
	}
	return false
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

// Promote implements [State]: the node handed off at block and is executing
// from the block after it, so it is ACTIVE, with the anchored root it matched
// at block as its verified anchor (executor spec, "Sync", step 6; #4385). A
// match alone never promotes: a node that matched and has not handed off
// executes nothing, and one that served from there served stale state to the
// next joiner (#4413).
func (s *PulledState) Promote(block uint64) {
	anchor := s.matched.Anchor
	if s.matched.Block != block {
		anchor = [32]byte{}
		for _, o := range s.tracker.Snapshot() {
			if o.Block == block {
				anchor = o.Anchor
				break
			}
		}
	}
	if anchor == ([32]byte{}) {
		// Not a state the join matched. Said out loud rather than promoted
		// on a root nothing anchored.
		s.log.Error("The node handed off at a block it holds no anchored root for; it stays BOOTING",
			"partition", s.partition, "block", block)
		return
	}
	if s.machine.PromoteToActive(anchor, block) {
		s.log.Info("This node is executing in agreement; it is ACTIVE",
			"partition", s.partition, "block", block)
	}
}

// Demote implements [State]: the node stopped executing in agreement at block,
// so its machine goes back to BOOTING and every service that asks it refuses
// again, and the gauge says so through the machine's OnChange (#4385). The
// tracker's streak starts again, so the next match takes as many as the first.
func (s *PulledState) Demote(block uint64) {
	if !s.machine.Demote(block) {
		return
	}
	s.tracker.ResetStreak()
	s.log.Warn("This node is not executing in agreement; it is BOOTING until it hands off again",
		"partition", s.partition, "block", block)
}

// Matched reports the block whose anchored root the local root equals. Until
// it does, the node keeps pulling: a root that matches is the only statement
// that the state this node holds is a block's state (executor spec, "Sync").
//
// It is the local root's block every time it is asked, not the block of the
// first match: a join that found a gap pulls on, so the state moves past the
// block it first matched, and answering with that block would settle staging
// against a state it is not (#4362). It changes nothing about the node's
// state: the join promotes when it hands off (Promote).
func (s *PulledState) Matched(ctx context.Context) (uint64, bool, error) {
	// The anchors first: the anchor of the block the state stands at is
	// collected as that block closes, and without this read the state would
	// be compared only with anchors read before the round that put it there.
	if err := s.readAnchors(ctx); err != nil && !(errors.Is(err, errors.NotReady) && s.spineStalled()) {
		s.log.Debug("This partition's anchors could not be read", "partition", s.partition, "error", err)
	}
	m, ok, err := s.tracker.Check(ctx)
	if err != nil {
		return 0, false, errors.UnknownError.Wrap(err)
	}
	if !ok {
		return 0, false, nil
	}
	s.matched = m
	s.provenAt = m.Block
	s.repairs = 0

	// The state is proven now, and with it the network definition it holds.
	s.refreshAuthority()
	return m.Block, true, nil
}
