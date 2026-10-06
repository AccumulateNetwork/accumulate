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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
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

// PulledState is the join's state half (#4293). It follows the algorithm of
// the executor spec, "Sync", "The algorithm", steps 1-3:
//
//  1. The pull takes the top of the peer's tree as of its block S, every
//     section's hash, and holds every section it does not already hold whole
//     as not loaded: one marker leaf whose value is the section's hash
//     (sections; executor spec, "Sync", §3a). Alongside it, it takes the
//     block-ledger record of every block after S, in block order, and pulls
//     every account each record names; an account it writes loads its
//     section first. A section's load never overwrites a value the records
//     wrote: an account a record has brought current is skipped, because the
//     section's page may be older than that record.
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
	// refusedAnswers counts the records answers refused (recordsOnce).
	refusedAnswers int

	partition *url.URL
	sources   Sources

	// db is this node's store, read and written past the section gate: the
	// join is what writes the sections. gated is the store the executor and
	// the node's services read, which the gate is set on (sections).
	db    *database.Database
	gated *database.Database

	// secs is the sections of the state tree this node has not loaded, and
	// the gate that loads one when a block reads it (#4485).
	secs *sections

	// overlap settles the first blocks after a match against the
	// partition's records of them (#4495). The section gate calls it.
	overlap *overlap

	// anchors is where this partition's verified roots come from: its own
	// anchors, collected from its validators (anchorsrc.Collector).
	anchors anchorSource

	// seedTry, if set, asks for the round consensus orders from; a pull
	// round calls it between its steps (SeedBetweenSteps, #4530).
	seedTry func(ctx context.Context)

	// wait is the step the join is on, said when it lasts (#4484).
	wait waiting

	// authority is the validator sets this node trusts. It is seeded from
	// this node's OWN store before anything is pulled, and it moves only
	// when the state has matched: <partition>/network and /globals are
	// pulled like every account, and what the pull writes is proven by the
	// match and by nothing before it (#4301, #4438).
	authority *anchorsrc.Authority

	machine *nodestate.Machine
	tracker *tracker.Tracker
	log     *slog.Logger

	// repairLog records the failures this node names and repairs (executor
	// spec, "Sync", §3b).
	repairLog *repairs

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

	// backfillMu guards headOnly, backfilled and backfillSaid: the
	// backfill runs on a goroutine of its own, beside the root watch and the
	// pull (Backfill; #4472).
	backfillMu sync.Mutex

	// headOnly are the accounts taken by their chain heads alone, whose
	// entries below the open mark set the node does not hold yet. Once the
	// state has matched they are backfilled while the node executes
	// (backfill). It is kept under SystemData too, and read back at start,
	// so a restart before the backfill ends does not forget them
	// (markHeadOnly). What each chain of them is missing is the account's
	// SyncedEntries record, its gaps (pull.Backfill); this set is the index
	// of the accounts whose record names a gap, so that finding them does not
	// read every account the partition holds (#4493).
	headOnly map[[32]byte]*url.URL

	// backfilled counts the accounts this process has backfilled, and
	// backfillSaid is when the backfill last said how far it had got (#4471).
	backfilled   int
	backfillSaid time.Time

	// ownRoot is whether the node is ACTIVE on a root no anchor verified: it
	// took the whole-restart exit or started with its network (Executing). agreedAt is the first block
	// after that whose root its signed anchor proved, said once (#4483).
	ownRoot bool
	// torn is StateOptions.Torn: the store was found torn at start.
	torn error
	// noOwnSaid is when Resumable last said the node has no block of its
	// own to resume from (#4508).
	noOwnSaid time.Time
	// peersHighest is the highest executed block a joining peer stated in
	// its refusal during the last peer round (#4501); lowerSaid is when
	// Resumable last said a peer holds a higher state.
	peersHighest uint64
	lowerSaid    time.Time
	agreedAt     atomic.Uint64

	// checked is executed, for the backfill's lines: the backfill reads it
	// beside the root watch, which writes executed.
	checked atomic.Uint64

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

	// peersJoining is whether the last round that asked the partition's
	// peers for their block found some refusing as joining, and fewer
	// serving than the partition's validators need to sign an anchor
	// (peersBlock; #4466). pulled is whether this process has started a
	// pull: from then its store holds a peer's accounts (Resumable).
	peersJoining bool
	pulled       bool

	// checkedHeld is whether this process has checked the entries the node
	// already holds on the accounts it takes whole for their messages
	// (pull.Options.CheckHeld). Once a process.
	checkedHeld bool

	// rootChains are the root chain positions and anchors of the blocks this
	// partition's validators signed anchors for (provenByBptChain).
	rootMu     sync.Mutex
	rootChains map[uint64]rootChainAt

	// mismatches are the accounts the last location found differing from
	// the peer's tree as of mismatchAt (locate).
	mismatchAt uint64
	mismatches []LeafDiff

	// anchorWait is how long a pull that stands complete at V holds there
	// for V's signed anchor before it goes on to the next V (Matched).
	anchorWait time.Duration
}

// syncing is one pull: the two cursors of the algorithm, and what the records
// have brought current.
type syncing struct {
	// start is S, the peer's block when the pull started.
	start uint64

	// last is L, the last block whose block-ledger record has been
	// processed. It starts at S.
	last uint64

	// topTaken is whether the top of the tree has been taken for this pull
	// (takeTop), and topAt the block it was taken as of.
	topTaken bool
	topAt    uint64

	// opened is every section this pull wrote into while it was not loaded,
	// and whether its load is done (loadOpened); sectionPages counts the
	// pages of sections it read.
	opened       map[sectionID]bool
	sectionPages int

	// current is every account a processed record named. A section's load
	// skips them: its page may be older than the record (step 1).
	current map[[32]byte]bool

	// retry is what a pull could not take and must take again: an account
	// a record named, or a leaf a section's page showed stale, that no peer served
	// this time. It is asked for again every round until it is taken or
	// every peer says it has no leaf for it.
	retry map[[32]byte]*url.URL

	// spine is whether the partition's spine has been taken whole once.
	spine bool

	// repair is whether this pull repairs what the node executed
	// (startRepair).
	repair bool

	// highest is the highest block any reply this pull took was read at: a
	// page of the records, the top, a section's page, an AccountStates answer. The state
	// is complete only once L has reached it, for until then an account read
	// at it may have changed at a block whose record is not yet processed
	// (executor spec, "Sync", §3).
	highest uint64

	// part is an account being answered in parts, over pages of one answer.
	part *statePart

	// servedBy names the peers whose answers this pull took.
	servedBy map[string]bool

	// chainsTaken are the accounts whose chains this pull took to an
	// answer's heads (pull.TakeChainsAt).
	chainsTaken map[[32]byte]bool

	// ready is whether the last round left the state complete at last, V:
	// the top is taken, every section opened is loaded, nothing is owed, and every record
	// through V is applied with nothing read after V left standing. The root
	// is compared with V's signed anchor then, and only then (Matched).
	ready bool

	// compared is the last V whose root was compared with its anchor, so
	// each V is compared once. waitFor is the V whose anchor the pull holds
	// for, since waitSince.
	compared  uint64
	waitFor   uint64
	waitSince time.Time
}

func newSyncing() *syncing {
	return &syncing{current: map[[32]byte]bool{}, retry: map[[32]byte]*url.URL{}, opened: map[sectionID]bool{}, servedBy: map[string]bool{}, chainsTaken: map[[32]byte]bool{}}
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
	// Torn is the finding of CheckStore when the node's store was found torn
	// at start: a chain's head names records the store does not hold. Such
	// a node never resumes onto its own block (Resumable is false); it joins
	// from its peers, and says why while it waits (#4507).
	Torn error

	// AnchorWait is how long a pull complete at a block V holds for V's
	// signed anchor before going on to the next V. Zero is DefaultAnchorWait.
	AnchorWait time.Duration

	// RepairLog is the file each failure the node names is appended to, for
	// an operator to repair offline what the node could not (executor spec,
	// "Sync", §3b). Empty, failures are only logged.
	RepairLog string

	Logger *slog.Logger
}

// DefaultAnchorWait is how long a complete pull holds for its block's signed
// anchor. A partition's validators sign a block's anchor as it closes, so a
// block's anchor is collected at quorum within a block or two of it.
const DefaultAnchorWait = 10 * time.Second

// NewState wires the pull, the anchor reader and the tracker together.
func NewState(opts StateOptions) (*PulledState, error) {
	if opts.Partition == nil || opts.Database == nil || opts.Sources == nil {
		return nil, errors.BadRequest.With("a join's state needs a partition, a database and peers to pull from")
	}
	machine := nodestate.New(opts.Partition)
	if opts.Torn != nil {
		mStoreTorn.WithLabelValues(opts.Partition.String()).Set(1)
	}
	track, err := tracker.New(opts.Database, opts.Partition)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	s := &PulledState{
		partition:  opts.Partition,
		db:         opts.Database.Ungated(),
		gated:      opts.Database,
		sources:    opts.Sources,
		machine:    machine,
		torn:       opts.Torn,
		tracker:    track,
		log:        log.With("module", "join"),
		repairLog:  newRepairs(log, opts.RepairLog),
		executed:   opts.ExecutedBlock,
		anchorWait: opts.AnchorWait,
	}
	if s.anchorWait <= 0 {
		s.anchorWait = DefaultAnchorWait
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
	// What an earlier process took by chain heads alone and did not
	// backfill before it stopped.
	if err := s.loadHeadOnly(); err != nil {
		return nil, errors.UnknownError.Wrap(err)
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
	collector.OnAsk = func(n uint64, who string) {
		if n == 0 {
			s.wait.on(stepAnchors, "asking", "the newest anchor number", "peer", who)
			return
		}
		s.wait.on(stepAnchors, "anchor", n, "validator", who)
	}
	collector.OnCall = func(n uint64, who string, took time.Duration) {
		if took < slowCall {
			return
		}
		what := "the newest anchor number"
		if n != 0 {
			what = fmt.Sprintf("anchor %d", n)
		}
		s.log.Warn("A call for this partition's anchors was slow", "partition", s.partition, "asked", what, "of", who, "took", took.Round(time.Millisecond))
	}
	collector.OnRefused = s.anchorRefused
	collector.OnRootChain = s.observeRootChain
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

	// The sections this node has not loaded, read from the markers its tree
	// holds, and the gate every batch of the node's store is checked
	// against: a block that reads one loads it, and every other reader is
	// refused (executor spec, "Sync", §3a; #4485).
	s.secs = newSections(opts.Partition, label, s.db, s.log)
	// The partition's peers, as the join finds them, but not through its
	// order: a block's load and the fill ask from goroutines of their own,
	// and the order is the join's.
	s.secs.peers = func(ctx context.Context) []pull.Source {
		srcs, _, err := s.sources.For(ctx, s.partition.JoinPath(protocol.Ledger))
		if err != nil {
			return nil
		}
		return srcs
	}
	s.secs.loaded = func(urls []*url.URL) { s.markHeadOnly(urls...) }
	// A peer that refuses a section it has not loaded names peers to ask
	// instead, and the loader asks them too (Redirection).
	if named, ok := s.sources.(interface {
		Source(peer.ID, *url.URL) pull.Source
	}); ok {
		s.secs.sourceFor = func(id peer.ID) pull.Source { return named.Source(id, s.partition) }
	}
	if err := s.secs.read(); err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	// The handoff is an overlap: the blocks after a match are settled
	// against their records, in the batch each executes in (§3a; #4495).
	// Every commit to the node's tree, by the executor and by the join, is
	// recorded in the write trail a failure is logged with (§3b).
	opts.Database.SetBPTWriteSink(s.repairLog)
	s.overlap = &overlap{partition: opts.Partition, label: label, log: s.log, repairs: s.repairLog, peers: s.secs.peers, ask: overlapAsk, blocks: OverlapBlocks}
	s.overlap.taken = func(urls ...*url.URL) { s.markHeadOnly(urls...) }
	s.secs.overlap = s.overlap
	s.overlap.loading = s.secs.AnyNotLoaded
	opts.Database.SetSectionGate(s.secs)

	mSpineStalled.WithLabelValues(label).Set(-1)
	nodestate.Report(label, machine.State())
	machine.OnChange(func(ad nodestate.Advertisement) {
		nodestate.Report(label, ad.State)
	})
	return s, nil
}

// The steps of a pull that ask peers, as the join says them when they last.
const (
	stepAnchors    = "reading this partition's signed anchors from its validators"
	stepPeersBlock = "asking this partition's peers which block it is at"
)

func (s *PulledState) joinWaiting() *waiting { return &s.wait }

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

// mBackfillLeft is how many accounts the join took by their chain heads alone
// and has not yet brought in whole: the history a node still lacks after it
// is ACTIVE (executor spec, "Past the match, every account taken by its heads
// is brought in whole"; #4471).
var mBackfillLeft = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "accumulate",
	Subsystem: "join",
	Name:      "backfill_accounts_left",
	Help:      "Accounts taken by their chain heads alone and not yet backfilled whole",
}, []string{"partition"})

// ResumeSlack is how far behind the highest state a peer states a node may
// stand and still resume from its own block after a whole restart: the bound
// on how far an executor lags its consensus position (primary
// DefaultMaxExecutionLag, 8 blocks), which is how far validators' executed
// blocks differ at a kill while their consensus positions coincide (#4501).
const ResumeSlack = 8

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
	s.wait.on(stepAnchors)
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
	round := newRoundTimer()
	defer round.report(s.log, s.partition)

	// The anchors first: they are what the match is judged against.
	//
	// A failure here ends the round, never the join. Every read is addressed
	// at a named peer, so any of them can fail for the ordinary reason that
	// the peer it picked is restarting -- and under chaos that is a
	// certainty. Returning an error reaches join.Run, which returns, and the
	// daemon then abandons the join for good.
	err := s.readAnchors(ctx)
	round.mark("anchors")
	if err != nil && !(errors.Is(err, errors.NotReady) && s.spineStalled()) {
		// A read held at an entry every peer refuses is the stall, said
		// once a minute by readAnchors, not once a round here.
		s.log.Info("This partition's anchors could not be read this round",
			"partition", s.partition, "error", err)
	}

	// A pull complete at V holds there, briefly, for V's signed anchor: the
	// root is compared at V once V's anchor is in (Matched). It is given up
	// on, and the next round's V is compared, once an anchor of a later
	// block has been collected without one for V -- a block that anchors
	// nothing -- or after AnchorWait.
	if p := s.sync; p != nil && p.ready && p.compared != p.last && p.waitFor == p.last &&
		s.clock().Sub(p.waitSince) < s.anchorWait {
		if _, ok := s.tracker.AnchorAt(p.last); ok || s.tracker.LatestObservedBlock() < p.last {
			return nil
		}
	}

	peer, err := s.peersBlock(ctx)
	round.mark("peers-block")
	s.wait.on("pulling the state")
	if err != nil {
		// No peer could say where the partition is. Nothing is lost by
		// waiting: the records are read from where they stopped.
		s.log.Info("No peer could say which block the partition is at this round", "partition", s.partition, "error", err)
		return nil
	}

	// Peers serve, but fewer than sign an anchor: the first of a partition
	// restarted as a whole to resume (#4466). A pull from them waits for an
	// anchor a quorum signed, and none will be signed until more of the
	// partition executes -- a joining validator signs nothing -- so a node
	// that can resume from its own block does, and pulls nothing. Once it
	// had pulled, its store would be no block's state and it could never
	// resume.
	if _, ok := s.Resumable(); ok {
		s.log.Info("Fewer of this partition's validators serve than sign an anchor; resuming from this node's own block rather than pulling from them",
			"partition", s.partition, "block", s.executed, "threshold", s.signingThreshold())
		return nil
	}

	// Recorded before the pull writes anything, where no pull reaches: a
	// store with part of a peer's state in it is no block's state, and must
	// not be resumed from as this node's own (Resumable; #4447).
	if s.sync == nil {
		err = s.markPullStarted()
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	// The anchors are read and the peers found: a node not seeded when its
	// join started is asked again here, and after each long step below.
	s.seedBetweenSteps(ctx)

	if s.sync == nil && s.repairFrom != 0 {
		s.startRepair(ctx)
	}
	if s.sync == nil {
		s.secs.forgetOpening()
		s.sync = newSyncing()
		s.sync.start, s.sync.last = peer.block, peer.block
		s.log.Info("Pulling the state: the top of the BPT, and every block-ledger record from here on",
			"partition", s.partition, "start", peer.block, "executed", s.executed)
	}
	p := s.sync
	p.ready = false

	// The top of the tree first: every section's hash as of a peer's block
	// at or after L, and every section the node does not hold whole not
	// loaded (executor spec, "Sync", §3a). What the pull writes after it
	// loads its section first.
	if !p.topTaken {
		if err := s.takeTop(ctx, p); err != nil {
			s.log.Info("The top of the peer's BPT could not be taken this round", "partition", s.partition, "error", err)
			return nil
		}
	}

	// The spine, whole, once: this partition's anchors, ledger, operators and
	// network definition, with their chains and the messages behind them
	// (§1, §3). It is pulled like the walk -- a record processed later pulls
	// again whatever of it a later block changed.
	if !p.spine {
		ok := true
		// The sections the node's own services read outside a block, so
		// none of them is refused once the node hands off.
		s.openFor(p, SystemAccounts(s.partition)...)
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

	// 1. The records: ChangedAccountStates after L, every page of it. Every
	// account it answers is written and marked current, and L moves to the
	// records' end T. Every account is marked current as the records name
	// it, whether or not it is taken this round, so the walk never writes
	// over it.
	round.mark("top+spine")
	s.seedBetweenSteps(ctx)
	s.processRecords(ctx, p, peer)
	round.mark("records")
	s.seedBetweenSteps(ctx)

	// 2. What an earlier round could not take, once each this round,
	// whoever named it.
	owed := p.retry
	p.retry = map[[32]byte]*url.URL{}
	if len(owed) > 0 {
		list := make([]*url.URL, 0, len(owed))
		for _, u := range owed {
			list = append(list, u)
		}
		sort.Slice(list, func(i, j int) bool { return list[i].String() < list[j].String() })
		s.takeAccounts(ctx, p, s.partitionSources(ctx), list)
	}

	round.mark("owed")

	// 3. The sections opened: each one's leaves, and every account they name
	// the records have not brought current.
	s.loadOpened(ctx, p)
	round.mark("sections")

	// With the top taken, every section opened loaded, nothing owed and L at
	// or above the highest block any reply was read at, every loaded section
	// is as of L, V, and every section not loaded holds the hash it had when
	// the top was taken, which no record after it changed. The root is
	// compared with V's signed anchor (Matched), and nothing is executed
	// before it matches (executor spec, "Sync", "The algorithm").
	p.ready = p.topTaken && openedLoaded(p) && len(p.retry) == 0 && p.last >= p.highest
	return nil
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
	p.start, p.last = from, from
	p.topTaken, p.spine, p.repair = true, true, true
	s.sync = p

	// A repair from the block ledger that did not bring a match is followed
	// by one that pulls the accounts again: the top of the tree taken again,
	// every section whose hash differs from the peers' not loaded, and loaded
	// again as it is needed (Paul, 2026-09-25: "If the pull of accounts doesn't
	// create a matching BPT root, then the joining node has to pull the
	// accounts again. I don't see what else it can do."). An account the
	// walk took wrongly, that no later block names, is beyond any record.
	s.repairs++
	if s.repairs > 1 {
		p.topTaken = false
		s.log.Info("A repair from the block ledger did not bring a match; taking the top of the tree again",
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

// maxRecordCallsPerRound bounds how many ChangedAccountStates pages one round
// asks for, so a partition far ahead cannot make one round endless. The next
// round goes on from L.
const maxRecordCallsPerRound = 64

// processRecords processes the block-ledger records after L (executor spec,
// "Sync", §3, step 1): ChangedAccountStates with After = L, every page of it,
// asked of the peer whose ledger named the lowest block -- the round's target
// -- and then of the others in the order of the blocks they named, so one
// peer naming a block far ahead does not carry L there (#4438 threat F3). An
// answer covers the records through its T, which the peer caps at its own
// block and at a span of records; L moves to T, and a peer with records past
// T is asked again from there, within the round's calls, until L reaches the
// round's target. It does not go past the target in one round: under load an
// answer of many pages ends at a view newer than its T, and chasing that view
// kept a join in its first round's records for as long as an answer took
// about as long as the blocks it covered (#4442, Docker 20260928T051402Z:
// the BVN pull never reached its walk). The next round goes on from L. A page
// that cannot be read stops the round's records there, and the next round
// asks again from L.
func (s *PulledState) processRecords(ctx context.Context, p *syncing, peer peersAt) {
	calls := 0
	for i := 0; i < len(peer.sources) && ctx.Err() == nil && calls < maxRecordCallsPerRound; {
		src := peer.sources[i]
		through, block, n, err := s.recordsOnce(ctx, p, src)
		calls += n
		switch {
		case err == nil:
		case errors.Is(err, errRefusedAnswer):
			// Nothing of it was written: another peer is asked.
			s.refusedAnswers++
			s.log.Info("A peer's answer of the block-ledger records was refused; the next is asked",
				"partition", s.partition, "peer", sourceKey(src), "after", p.last, "error", err)
			i++
			continue
		case n <= 1:
			// The first page was not served: another peer is asked.
			s.log.Info("A peer did not answer the block-ledger records; the next is asked",
				"partition", s.partition, "peer", sourceKey(src), "after", p.last, "error", err)
			i++
			continue
		default:
			s.log.Info("A page of the block-ledger records could not be read; the records go on from here next round",
				"partition", s.partition, "peer", sourceKey(src), "after", p.last, "error", err)
			return
		}
		p.last = through
		if through < block {
			// The answer was cut short by the peer's cap on blocks per
			// answer. The walk is fast enough that the records after L
			// should be a few blocks: the join has fallen behind (#4461).
			s.log.Info("The block-ledger records after L were more blocks than one answer covers; the join has fallen behind",
				"partition", s.partition, "peer", sourceKey(src), "through", through, "peer-block", block)
		}
		if through >= block || through >= peer.block {
			return
		}
	}
}

// RefusedRecordAnswers is how many answers of the block-ledger records this
// join has refused. An honest peer's answer is never refused.
func (s *PulledState) RefusedRecordAnswers() int { return s.refusedAnswers }

// errRefusedAnswer is an answer of the records refused whole: nothing of it
// is written, and the next peer is asked.
var errRefusedAnswer = errors.Conflict.With("the answer was refused")

// recordsOnce asks one peer for one answer of ChangedAccountStates after L,
// every page of it, and takes it: the block-ledger records it carries, and
// every account they name (#4461). The answer is taken whole or not at all.
// Its pages are held until the last, and the records must name exactly the
// accounts the pages serve: an answer that leaves out an account a record
// names, or serves one no record names, is refused, nothing of it is written,
// and the next peer is asked. The accounts of a taken answer are marked
// current by the records' names and written in one commit. It returns the
// records' end T, the block the last page was read at, and how many pages it
// asked for.
func (s *PulledState) recordsOnce(ctx context.Context, p *syncing, src pull.Source) (through, block uint64, calls int, err error) {
	after := p.last
	var cursor []byte
	var records []*api.BlockLedgerRecord
	var states []*api.AccountState
	var blocks []uint64
	answered := map[[32]byte]bool{}
	highest := uint64(0)
	p.part = nil
	for {
		calls++
		rec, err := src.QueryChangedAccountStates(ctx, s.partition, &api.ChangedAccountStatesQuery{After: after, Cursor: cursor})
		if err != nil {
			return 0, 0, calls, errors.UnknownError.Wrap(err)
		}
		if rec == nil {
			return 0, 0, calls, errors.Conflict.With("the peer answered nothing")
		}
		if rec.Through < after || rec.Block < rec.Through || cursor != nil && rec.Through != through {
			return 0, 0, calls, errors.Conflict.WithFormat("the peer answered the records after %d through %d at block %d", after, rec.Through, rec.Block)
		}
		if cursor == nil {
			records = rec.Records
		}
		through, block = rec.Through, rec.Block
		highest = max(highest, block)
		if rec.Part != nil && rec.Part.Url != nil {
			answered[accountKey(rec.Part.Url)] = true
		}
		for _, st := range s.statesOf(p, rec) {
			if st.Url == nil {
				continue
			}
			answered[accountKey(st.Url)] = true
			states = append(states, st)
			blocks = append(blocks, rec.Block)
		}
		if len(rec.Cursor) == 0 {
			break
		}
		cursor = rec.Cursor
	}

	named, err := namedByRecords(records, after, through)
	switch {
	case err != nil:
	default:
		err = coversExactly(named, answered)
	}
	if err != nil {
		return 0, 0, calls, fmt.Errorf("%w: records after %d through %d: %w", errRefusedAnswer, after, through, err)
	}

	p.highest = max(p.highest, highest)
	opening := make([]*url.URL, 0, len(named))
	for _, u := range named {
		opening = append(opening, u)
	}
	s.openFor(p, opening...)
	for k, u := range named {
		p.current[k] = true
		if !answered[k] {
			p.retry[k] = u
		}
	}
	taken := s.takeStates(ctx, p, src, states, blocks)
	for _, st := range states {
		if !taken[accountKey(st.Url)] {
			p.retry[accountKey(st.Url)] = st.Url
		}
	}
	return through, block, calls, nil
}

// namedByRecords is every account the records name, by the rule the peer
// answers them by (api.RecordNames). A record of a block outside the
// answer's span, or two of one block, is refused.
func namedByRecords(records []*api.BlockLedgerRecord, after, through uint64) (map[[32]byte]*url.URL, error) {
	named := map[[32]byte]*url.URL{}
	seen := map[uint64]bool{}
	for _, r := range records {
		if r == nil {
			return nil, errors.Conflict.With("an empty record")
		}
		if r.Index <= after || r.Index > through || seen[r.Index] {
			return nil, errors.Conflict.WithFormat("a record of block %d, outside %d through %d or repeated", r.Index, after+1, through)
		}
		seen[r.Index] = true
		for _, u := range api.RecordNames(r.Entries) {
			named[accountKey(u)] = u
		}
	}
	return named, nil
}

// coversExactly says whether the accounts an answer served are exactly those
// its records name.
func coversExactly(named map[[32]byte]*url.URL, answered map[[32]byte]bool) error {
	for k, u := range named {
		if !answered[k] {
			return errors.Conflict.WithFormat("a record names %v and the answer does not serve it", u)
		}
	}
	for k := range answered {
		k := k
		if _, ok := named[k]; !ok {
			return errors.Conflict.WithFormat("the answer serves an account no record names (%x)", k[:8])
		}
	}
	return nil
}

// statePart is an account answered in parts: the bytes of its marshalled
// state so far.
type statePart struct {
	url  *url.URL
	data []byte
}

// statesOf is the accounts one page answers whole, with an account answered
// in parts once its last part has arrived. The parts come from different
// views; the whole is checked against its leaf like any other.
func (s *PulledState) statesOf(p *syncing, rec *api.AccountStatesRecord) []*api.AccountState {
	out := rec.States
	pt := rec.Part
	if pt == nil {
		p.part = nil
		return out
	}
	if pt.Offset == 0 || p.part == nil || !p.part.url.Equal(pt.Url) || uint64(len(p.part.data)) != pt.Offset {
		p.part = &statePart{url: pt.Url}
		if pt.Offset != 0 {
			// A part that does not continue what was held: the account
			// is owed.
			p.retry[accountKey(pt.Url)] = pt.Url
			p.part = nil
			return out
		}
	}
	p.part.data = append(p.part.data, pt.Data...)
	if uint64(len(p.part.data)) < pt.Total {
		return out
	}
	st := new(api.AccountState)
	err := st.UnmarshalBinary(p.part.data)
	p.part = nil
	if err != nil || st.Url == nil || !st.Url.Equal(pt.Url) {
		s.log.Info("An account served in parts did not reassemble; it is asked for again", "account", pt.Url, "error", err)
		p.retry[accountKey(pt.Url)] = pt.Url
		return out
	}
	return append(append([]*api.AccountState(nil), out...), st)
}

// takeStates writes the accounts one answer serves, or hands each to the
// pull that takes it another way, and returns those whose answer was taken. An
// answer refused is that peer's: the account is asked of the next peer, or
// owed. Every account written by its served state is written in one commit
// (pull.KeepStates), and what is left to backfill is recorded in one more: a
// commit per account made a walk page of 256 accounts cost 22 s on
// BlockchainDB (#4461).
//
// The trust chain (pull.WholeAccounts) and the windowed accounts
// (pull.WindowAccounts) are taken with their chains' entries and the
// messages behind them, by pullOne, and so is every account of a repair
// (taken whole). pullOne reads the account as the peer holds it now, a block
// after the answer's; the answer's state is written over it, so the account
// stands at the answer's block like every other (#4442; #4453 review F1).
// An account the answer holds no leaf for is deleted: at the answer's block
// it does not exist, and a record after it names it if a block creates it.
// That is the answer of a peer whose section is loaded, and one such answer
// is the peer's word as a served state is, which the match proves (executor
// spec, "Sync", §3a; #4496). An account in a section the peer has not loaded
// (NotLoaded) is no verdict at all: nothing is deleted, and it is asked of
// the next peer. Every other account is checked against its leaf and written
// as served (pull.KeepStates).
//
// blocks[i] is the block of the view states[i] was read from.
func (s *PulledState) takeStates(ctx context.Context, p *syncing, src pull.Source, states []*api.AccountState, blocks []uint64) map[[32]byte]bool {
	taken := map[[32]byte]bool{}
	type keeping struct {
		st              *api.AccountState
		block           uint64
		whole, windowed bool
	}
	var keep []keeping
	opening := make([]*url.URL, 0, len(states))
	for _, st := range states {
		if st.Url != nil {
			opening = append(opening, st.Url)
		}
	}
	s.openFor(p, opening...)
	for i, st := range states {
		block := blocks[i]
		u := st.Url
		if u == nil {
			continue
		}
		if !Routable(u) {
			taken[accountKey(u)] = true
			continue
		}
		if !s.ours(ctx, u) {
			s.log.Info("A named account is not this partition's and was dropped",
				"account", u, "partition", s.partition)
			taken[accountKey(u)] = true
			continue
		}
		if st.NotLoaded {
			s.log.Debug("A peer has not loaded the section an account is in; it is asked of another",
				"account", u, "peer", sourceKey(src), "section", st.Section)
			continue
		}
		if st.NoLeaf {
			s.forget(u)
			delete(p.retry, accountKey(u))
			taken[accountKey(u)] = true
			continue
		}
		// The chains' entries and the messages behind them, to the answer's
		// heads and no further (pull.TakeChainsAt).
		window, windowed := pull.WindowAccounts(s.partition)[strings.ToLower(u.String())]
		whole := s.takenWhole(u)
		if whole || windowed {
			srcs, _, err := s.sourcesFor(ctx, u)
			if err == nil {
				srcs = append([]pull.Source{src}, s.order(srcs)...)
				k := accountKey(u)
				_, err = pull.TakeChainsAt(ctx, srcs, s.db, st, block, whole, p.chainsTaken[k], window)
				if err == nil {
					p.chainsTaken[k] = true
				}
			}
			if err != nil {
				s.log.Info("An account's chains could not be taken to the answer's heads; it is asked for again",
					"account", u, "block", block, "error", err)
				continue
			}
		}
		keep = append(keep, keeping{st, block, whole, windowed})
	}

	list := make([]*api.AccountState, len(keep))
	asOf := make([]uint64, len(keep))
	for i, k := range keep {
		list[i], asOf[i] = k.st, k.block
	}
	var at uint64
	if s.sync != nil {
		at = s.sync.last
	}
	errs := pull.KeepStates(taggedStore{s.db, "keep", at}, list, asOf)
	var headOnly []*url.URL
	for i, k := range keep {
		u := k.st.Url
		if errs[i] != nil {
			s.log.Info("A peer's answer for an account was refused; it is asked of another",
				"account", u, "peer", sourceKey(src), "error", errs[i])
			continue
		}
		if err := pull.KeepQueuedMessages(ctx, src, s.db, k.st); err != nil {
			s.log.Info("A queued delivery's message could not be taken; the account is asked of another",
				"account", u, "peer", sourceKey(src), "error", err)
			continue
		}
		delete(p.retry, accountKey(u))
		p.servedBy[sourceKey(src)] = true
		taken[accountKey(u)] = true
		if !k.whole {
			// Taken by its heads, or its window: the rest is backfilled once
			// the state has matched, in a repair too (#4519).
			headOnly = append(headOnly, u)
		}
	}
	s.markHeadOnly(headOnly...)
	return taken
}

// accountRouter is how the join tells an account's partition without asking
// for its peers (QueryPeers.PartitionOf).
type accountRouter interface {
	PartitionOf(*url.URL) (*url.URL, error)
}

// ours is whether u is this partition's account. A peer's answer names
// accounts, and a partition's store holds no account of another's (see
// sourcesFor).
func (s *PulledState) ours(ctx context.Context, u *url.URL) bool {
	if r, ok := s.sources.(accountRouter); ok {
		partition, err := r.PartitionOf(u)
		return err == nil && partition.Equal(s.partition)
	}
	_, _, err := s.sourcesFor(ctx, u)
	return !errors.Is(err, errNotThisPartition)
}

// statesChunk is how many accounts one AccountStates call asks for when the
// calls run across the peers, and statesInFlight how many run at once against
// each peer: a validator admits four queries at a time (api/v3 queryGate). A
// validator serves a state in ~17 ms, I/O in its store; asked of one peer one
// call after another, the stale accounts of a round against a tree of 29k
// accounts took longer than the blocks the round covered, and the pull chased
// the head for sixteen minutes (#4535, run 20261005T185718Z).
const statesChunk = 64
const statesInFlight = 4

// statesAnswer is what one chunk's calls brought: the peer that served it
// and its answers in order, or no answers when no peer served it.
type statesAnswer struct {
	chunk []*url.URL
	src   pull.Source
	recs  []*api.AccountStatesRecord
}

// takeAccounts asks the sources for the accounts' states -- statesChunk
// accounts a call, statesInFlight calls at a time against each source, a
// chunk a source does not serve asked of the next -- and keeps every state
// served (takeStates), in the order the answers arrive. An account no source
// served is owed to the next round.
func (s *PulledState) takeAccounts(ctx context.Context, p *syncing, srcs []pull.Source, accounts []*url.URL) {
	var batchable []*url.URL
	for _, u := range accounts {
		switch {
		case ctx.Err() != nil:
			p.retry[accountKey(u)] = u
		case !Routable(u):
		default:
			batchable = append(batchable, u)
		}
	}
	if len(batchable) == 0 {
		return
	}
	if len(srcs) == 0 {
		for _, u := range batchable {
			p.retry[accountKey(u)] = u
		}
		return
	}
	var chunks [][]*url.URL
	for len(batchable) > 0 {
		n := min(len(batchable), statesChunk)
		chunks = append(chunks, batchable[:n])
		batchable = batchable[n:]
	}

	// The calls run on workers; the answers are kept here, on the round's
	// goroutine, which alone touches p.
	start, asOf := p.start, p.last
	next := make(chan int)
	out := make(chan statesAnswer)
	workers := min(statesInFlight*len(srcs), len(chunks))
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				src, recs := s.askStatesOf(ctx, srcs, i%len(srcs), chunks[i], start, asOf)
				out <- statesAnswer{chunks[i], src, recs}
			}
		}()
	}
	go func() {
		for i := range chunks {
			select {
			case next <- i:
			case <-ctx.Done():
			}
			if ctx.Err() != nil {
				break
			}
		}
		close(next)
		wg.Wait()
		close(out)
	}()
	for a := range out {
		served := s.keepAnswers(ctx, p, a.src, a.chunk, a.recs)
		for _, u := range a.chunk {
			if !served[accountKey(u)] {
				p.retry[accountKey(u)] = u
			}
		}
	}
}

// askStatesOf asks the sources, from the one at turn, for a chunk's states
// until one serves it: every page of the answer, as of asOf (zero: current).
// It reads nothing of p: it runs beside the round.
func (s *PulledState) askStatesOf(ctx context.Context, srcs []pull.Source, turn int, chunk []*url.URL, start, asOf uint64) (pull.Source, []*api.AccountStatesRecord) {
	for k := range srcs {
		if ctx.Err() != nil {
			return nil, nil
		}
		src := srcs[(turn+k)%len(srcs)]
		if recs, ok := s.statesFrom(ctx, src, chunk, start, asOf); ok {
			return src, recs
		}
	}
	return nil, nil
}

// statesFrom asks one source for a chunk's states, every page of the answer,
// and says whether it served them.
func (s *PulledState) statesFrom(ctx context.Context, src pull.Source, chunk []*url.URL, start, asOf uint64) ([]*api.AccountStatesRecord, bool) {
	var recs []*api.AccountStatesRecord
	var cursor []byte
	for ctx.Err() == nil {
		rec, err := src.QueryAccountStates(ctx, s.partition, &api.AccountStatesQuery{Accounts: chunk, Cursor: cursor, ForHeight: asOf})
		if err != nil && asOf != 0 && cursor == nil {
			// A peer that cannot answer as of L answers its current
			// state, of a block past L, and the records catch up to it.
			asOf = 0
			rec, err = src.QueryAccountStates(ctx, s.partition, &api.AccountStatesQuery{Accounts: chunk})
		}
		if err != nil {
			s.log.Info("A peer did not answer for accounts; they are asked of another",
				"partition", s.partition, "peer", sourceKey(src), "accounts", len(chunk), "error", err)
			return nil, false
		}
		if rec.Block < start {
			s.log.Info("A peer answered for accounts at a block before the pull's start; they are asked of another",
				"partition", s.partition, "peer", sourceKey(src), "block", rec.Block, "start", start)
			return nil, false
		}
		recs = append(recs, rec)
		if len(rec.Cursor) == 0 {
			return recs, true
		}
		cursor = rec.Cursor
	}
	return nil, false
}

// keepAnswers keeps the states one chunk's answers carry and reports the
// accounts kept. The answers are one source's pages in order, so an account
// served in parts reassembles (statesOf).
func (s *PulledState) keepAnswers(ctx context.Context, p *syncing, src pull.Source, chunk []*url.URL, recs []*api.AccountStatesRecord) map[[32]byte]bool {
	served := map[[32]byte]bool{}
	inChunk := make(map[[32]byte]bool, len(chunk))
	for _, u := range chunk {
		inChunk[accountKey(u)] = true
	}
	p.part = nil
	for _, rec := range recs {
		p.highest = max(p.highest, rec.Block)
		var states []*api.AccountState
		for _, st := range s.statesOf(p, rec) {
			if st.Url != nil && inChunk[accountKey(st.Url)] {
				states = append(states, st)
			}
		}
		blocks := make([]uint64, len(states))
		for i := range blocks {
			blocks[i] = rec.Block
		}
		for k := range s.takeStates(ctx, p, src, states, blocks) {
			served[k] = true
		}
	}
	return served
}

// partitionSources is the partition's peers, each reached by name, in the
// order an account is asked of them (order).
func (s *PulledState) partitionSources(ctx context.Context) []pull.Source {
	srcs, _, err := s.sources.For(ctx, s.partition.JoinPath(protocol.Ledger))
	if err != nil {
		s.log.Info("The partition's peers could not be found this round", "partition", s.partition, "error", err)
		return nil
	}
	return s.order(srcs)
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
// The trust chain is taken whole, with every chain entry and the message
// behind each (pull.WholeAccounts); the anchor pool, the ledger and the
// synthetic ledger by the recent window the executor reads of them
// (pull.WindowAccounts, #4452); every other account state-only. What is not
// taken whole is backfilled after the match. Nothing is verified against a root here:
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

	// A repair takes an account as the join takes any: syncing never
	// backfills (executor spec, "Sync", "Two mismatches", 2; #4519).
	srcs = s.order(srcs)

	window, windowed := pull.WindowAccounts(s.partition)[strings.ToLower(u.String())]
	whole := s.takenWhole(u)
	mode := pull.ModeStateOnly
	switch {
	case whole:
		mode = pull.ModeFullSpine
	case windowed:
		// A repair takes these by their window too: what the node
		// executed wrongly is recent, and the history below is the
		// peers', brought in by the backfill.
		mode = pull.ModeWindow
	}
	// No batch is begun here: the pull begins the account's once its chains
	// have streamed, and a view open across the stream pins every page's
	// pre-images on the BlockchainDB backend (#4446).
	pending, served, err := pull.FetchFrom(ctx, srcs, nil, u, pull.Options{
		Mode:      mode,
		Window:    window,
		Partition: partition,
		// The answer with a receipt is the one that carries the rest of the
		// leaf beside the body (#4399), and the one whose NotFound says the
		// peer holds no leaf (#4397).
		WithReceipt: true,
		// Once a process, every account taken whole is checked for what an
		// earlier join left without its message -- the synthetic ledger
		// included, which every join before #4434 took state-only.
		CheckHeld: (whole || windowed) && !s.checkedHeld,
		// A node that has executed since its last match repairs what it
		// executed: a chain of its own longer than the peer's is taken again.
		RetakeLonger: p.repair,
		// The entries under a chain's head go to the database a page at a
		// time, so a pull's memory does not grow with the history (#4446).
		Store: s.db,
	})
	if err == nil && served >= 0 && served < len(srcs) {
		p.servedBy[sourceKey(srcs[served])] = true
	}
	if err == nil {
		// Read as the peer holds it now: the state is complete only once
		// the records have reached this block (Pull).
		p.highest = max(p.highest, pending.Block)
	}
	defer func() {
		// What was taken by its heads alone is backfilled once the state
		// has matched (backfill). A whole pull goes on from the entries the
		// node holds, so an account taken by its heads before stays to be
		// backfilled below them.
		if err != nil || whole {
			return
		}
		if windowed {
			// Backfilled after the match, never while the node waits on
			// it; and only when the window left something below it that
			// the node does not hold.
			if pending.Windowed {
				s.markHeadOnly(u)
			}
			return
		}
		s.markHeadOnly(u)
	}()
	switch {
	case err == nil:
	case stderrors.Is(err, pull.ErrNoLeaf):
		// Dropped, not owed (#4397): every source was asked and every one
		// answered that its tree holds no leaf for the name. Asking again
		// changes nothing, and a record names it again if a block ever
		// gives it one.
		s.openFor(p, u)
		s.forget(u)
		return dropped
	default:
		s.log.Info("An account could not be pulled; it is asked for again next round", "account", u, "error", err)
		p.retry[accountKey(u)] = u
		return owed
	}

	s.openFor(p, u)

	// Keep updates the BPT, then commits: Batch.Commit commits the BPT store
	// and never calls Account.putBpt, so a perfectly pulled account left the
	// local root exactly where it was and the match could never come (#4305).
	err = pending.Keep()
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

// peersAt is where the partition's peers stand: the round's target, and the
// peers that named a block at or after it, the one that named the lowest
// first.
type peersAt struct {
	block   uint64
	sources []pull.Source
}

// peersBlock is the block the partition's peers stand at: the LOWEST block any
// of them names. One peer's word is not a target the node must reach -- a
// peer that named a block far ahead would set S there and hold the records
// off, or carry L there (#4438 threat F3) -- so the records are asked of the
// peer that named the lowest first (processRecords).
//
// The lowest is floored at the newest block a quorum of the partition's
// validators signed an anchor for, as this node has verified it: the
// partition has certainly reached that block, so a peer that names a block
// before it is behind -- a laggard, or one answering 0 -- and is not counted
// (#4438 re-review F-2). Otherwise one such peer would pin the target, and
// with it L, where it stands.
func (s *PulledState) peersBlock(ctx context.Context) (peersAt, error) {
	s.peersJoining = false
	s.wait.on(stepPeersBlock)
	srcs, _, err := s.sources.For(ctx, s.partition.JoinPath(protocol.Ledger))
	if err != nil {
		return peersAt{}, errors.UnknownError.WithFormat("find the partition's peers: %w", err)
	}
	floor := s.tracker.LatestObservedBlock()
	type named struct {
		src   pull.Source
		block uint64
	}
	var at []named
	var last error
	var joining, other int
	var highest uint64
	defer func() {
		s.peersJoining = joining > 0 && uint64(other) < s.signingThreshold()
		s.peersHighest = highest
	}()
	for _, src := range srcs {
		s.wait.on(stepPeersBlock, "peer", src)
		n, err := ledgerIndexOf(ctx, src, s.partition)
		switch {
		case nodestate.IsJoiningRefusal(err):
			joining++
			if n, ok := nodestate.JoiningExecuted(err); ok && n > highest {
				highest = n
			}
		case errors.Is(err, errors.NotReady), err == nil:
			// A peer that answered: with its ledger, or busy. Either is a
			// peer that is not joining.
			other++
		}
		if err != nil {
			last = err
			continue
		}
		if n < floor {
			last = errors.NotReady.WithFormat("%v stands at block %d, before the newest signed anchor's block %d", src, n, floor)
			continue
		}
		at = append(at, named{src, n})
	}
	if len(at) == 0 {
		return peersAt{}, errors.UnknownError.WithFormat("no peer served its ledger: %w", last)
	}
	sort.SliceStable(at, func(i, j int) bool { return at[i].block < at[j].block })
	out := peersAt{block: at[0].block}
	for _, a := range at {
		out.sources = append(out.sources, a.src)
	}
	return out, nil
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

// signingThreshold is how many of this partition's validators sign an anchor,
// in the set this node trusts; one when it names none, so that a peer serving
// is then, as before #4466, one to join from.
func (s *PulledState) signingThreshold() uint64 {
	id, ok := protocol.ParsePartitionUrl(s.partition)
	if !ok || s.authority == nil {
		return 1
	}
	set, err := s.authority.SetFor(id)
	if err != nil || set.Threshold == 0 {
		return 1
	}
	return set.Threshold
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

// Resumable implements [State]: the block this node executed itself, when it
// may execute from there with nothing matched — the partition restarted as a
// whole (executor spec, "Sync", "A partition that restarts as a whole";
// #4447). That is so when both hold:
//
//   - the last round found peers refusing as joining, and fewer serving or
//     busy than the partition's threshold of anchor signers (#4466). An
//     unreachable peer says nothing either way. Peers that serve are ones to
//     join from only when enough of them serve to sign the anchor a join
//     matches against: a joining validator signs nothing, so with fewer, no
//     anchor after the restart reaches its quorum until more resume. The
//     first validators of a partition restarted as a whole to resume serve
//     again, and are as few as the race made them.
//   - the store is this node's own execution through that block: no pull has
//     started, in this process or in one before it that did not hand off
//     (PullStarted), and the record the executor writes with every block
//     names the block the ledger does.
func (s *PulledState) Resumable() (uint64, bool) {
	if s.torn != nil {
		// A torn store is no block's state: the head of some chain names
		// records the store does not hold, and a node that resumed onto it
		// failed every block it produced (#4507). It joins instead.
		return 0, false
	}
	if !s.peersJoining || s.pulled || s.sync != nil || s.repairFrom != 0 || s.executed == 0 {
		return 0, false
	}
	if s.executed <= protocol.GenesisBlock {
		// Genesis is not a block of this node's own (#4304). A node here has
		// found fewer validators serving than sign an anchor and holds
		// nothing to resume from: no state can be proven to it until more
		// of them execute. It keeps collecting and says so, once a minute,
		// rather than taking an exit it cannot complete (run
		// 20261002T145702Z, 17:18Z: "no consensus checkpoint matched block
		// 1", every round; #4508).
		s.sayMu.Lock()
		if now := s.clock(); now.Sub(s.noOwnSaid) >= stallSayEvery {
			s.noOwnSaid = now
			s.log.Info("Fewer of this partition's validators serve than sign an anchor, and this node has no block of its own to resume from: no state can be proven to it; collecting",
				"partition", s.partition, "block", s.executed)
		}
		s.sayMu.Unlock()
		return 0, false
	}
	if s.peersHighest > s.executed+ResumeSlack {
		// A peer holds a state further ahead than consensus can replay to
		// this node from its own checkpoint: a node that resumed here would
		// produce the blocks after its own as if the ones it missed did not
		// exist, and end at the peers' height with another root
		// (TestAWholeRestartResumesFromTheHighestState). It waits for the
		// peers holding the highest to resume, and joins from them
		// (executor spec, "resumes from its highest signed, validated
		// state"; #4501). Within ResumeSlack it resumes: that gap is the
		// executors' lag behind one consensus position at the kill, and the
		// groups after its checkpoint are replayed to it as to any node
		// that resumes -- and a peer that has resumed already states a
		// block that moves on.
		s.sayMu.Lock()
		if now := s.clock(); now.Sub(s.lowerSaid) >= stallSayEvery {
			s.lowerSaid = now
			s.log.Info("A peer holds a higher state than this node's own; not resuming from this node's block, waiting to join from the peers that hold the highest",
				"partition", s.partition, "block", s.executed, "peers-highest", s.peersHighest)
		}
		s.sayMu.Unlock()
		return 0, false
	}
	id, ok := protocol.ParsePartitionUrl(s.partition)
	if !ok {
		return 0, false
	}
	batch := s.db.Begin(false)
	defer batch.Discard()
	started, err := batch.SystemData(id).PullStarted().Get()
	switch {
	case errors.Is(err, errors.NotFound):
	case err != nil || started != 0:
		return 0, false
	}
	executed, err := batch.SystemData(id).ExecutedBlock().Get()
	if err != nil || executed != s.executed {
		return 0, false
	}
	ledger, err := readExecutedBlock(s.db, s.partition)
	if err != nil || ledger != s.executed {
		return 0, false
	}
	return s.executed, true
}

// markPullStarted records that a pull is about to write a peer's state into
// this store (Resumable).
func (s *PulledState) markPullStarted() error {
	s.pulled = true
	// Zero is "no pull"; a node that had executed nothing still started one.
	return s.writePullStarted(max(s.executed, 1))
}

// writePullStarted writes the PullStarted record: the block the node had
// executed when a pull began, or zero once it handed off.
func (s *PulledState) writePullStarted(block uint64) error {
	id, ok := protocol.ParsePartitionUrl(s.partition)
	if !ok {
		return nil
	}
	batch := s.db.Begin(true)
	defer batch.Discard()
	err := batch.SystemData(id).PullStarted().Put(block)
	if err == nil {
		err = batch.Commit()
	}
	if err != nil {
		return errors.UnknownError.WithFormat("record that a pull started: %w", err)
	}
	return nil
}

// Executing implements [State]: the node handed off at block, its own last
// block, because its partition restarted as a whole (Resumable). It is ACTIVE
// on its own root: no peer had a root to verify it against, and every one of
// them is taking the same exit. Its services answer again from here; leaving
// it BOOTING would leave every validator of the partition refusing the others
// the anchors the root watch checks its blocks against (#4295, #4447).
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
	s.ownRoot = true
	return nil
}

// AgreedAt is the first block, after the node became ACTIVE on its own root,
// whose root its signed anchor proved; zero until then, and for a node that
// joined, whose promotion is its agreement (#4483).
func (s *PulledState) AgreedAt() uint64 { return s.agreedAt.Load() }

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
	s.secs.dropReady()
	s.overlap.stop(block)
	if !s.machine.Demote(block) {
		return
	}
	s.tracker.ResetStreak()
	s.log.Warn("This node is not executing in agreement; it is BOOTING until it hands off again",
		"partition", s.partition, "block", block)
}

// Matched compares the local root with the signed anchor of block V, once the
// pull stands complete at V: the walk has covered the tree, nothing is owed,
// and every block-ledger record through V is applied, with nothing read after
// V left standing (Pull). Then the local state is block V's, and its root must
// be V's anchored root (executor spec, "Sync", "The algorithm", step 3; Paul,
// 2026-09-28: "once the BPT is caught up, and once all the blockledger
// accounts are updated, THAT block should match the anchor and BPT hash").
//
// A match is the proof, and the join hands off at V: V + 1 executes from the
// consensus buffer. Nothing is executed before it: no block n executes until
// the tree is complete and every record through n - 1 is applied.
//
// V's anchor is collected at quorum as V closes; until it is in, the pull
// holds at V (Pull), for AnchorWait at most, and then goes on to the next
// round's V. A mismatch is a pull bug: the accounts whose leaves differ from
// the peer's tree as of V are found and named (locate), and taken again, and
// the root is compared at the next V. Each V is compared once.
//
// It changes nothing about the node's state: the join promotes when it hands
// off (Promote).
func (s *PulledState) Matched(ctx context.Context) (uint64, bool, error) {
	// The anchors first: V's is collected as V closes.
	if err := s.readAnchors(ctx); err != nil && !(errors.Is(err, errors.NotReady) && s.spineStalled()) {
		s.log.Debug("This partition's anchors could not be read", "partition", s.partition, "error", err)
	}
	s.wait.on("proving the pulled state against its signed anchor")
	p := s.sync
	if p == nil || !p.ready || p.compared == p.last {
		return 0, false, nil
	}
	v := p.last
	batch := s.db.Begin(false)
	local, err := batch.GetBptRootHash()
	batch.Discard()
	if err != nil {
		return 0, false, errors.UnknownError.WithFormat("read the local root: %w", err)
	}

	anchor, ok := s.tracker.AnchorAt(v)
	if !ok {
		// A block that sends no anchor of its own -- a partition at rest
		// anchors at most every fourth block -- or whose anchor is not
		// signed yet: its root is proven by a later block's signed root
		// chain, if one has been collected (provenByBptChain).
		anchor, ok = s.rootOfUnanchored(ctx, v, local)
	}
	if !ok {
		if p.waitFor != v {
			p.waitFor, p.waitSince = v, s.clock()
			s.log.Info("The state is complete at a block; waiting for its signed anchor",
				"partition", s.partition, "block", v, "latest-anchor", s.tracker.LatestObservedBlock())
		}
		return 0, false, nil
	}
	p.compared = v
	if local != anchor {
		s.log.Warn("The state complete at a block does not have its signed root; locating the accounts that differ",
			"partition", s.partition, "block", v, "local", fmt.Sprintf("%x", local), "anchor", fmt.Sprintf("%x", anchor))
		s.locate(ctx, p, v)
		return 0, false, nil
	}

	s.log.Info("The state is proven at the block it is complete at",
		"partition", s.partition, "block", v, "root", fmt.Sprintf("%x", anchor))
	s.matched = tracker.Match{Block: v, Anchor: anchor}
	s.provenAt = v
	s.repairs = 0

	// The blocks after V are the overlap's: it begins before the handoff
	// executes the buffered groups, which are the blocks most likely to
	// load a section ahead of themselves (§3a; #4495).
	s.overlap.start(v)

	// The state is proven now, and with it the network definition it holds.
	s.refreshAuthority()
	return v, true, nil
}

// RecordsThrough is the last block whose block-ledger record the pull in
// progress has applied, and zero when no pull is in progress.
func (s *PulledState) RecordsThrough() uint64 {
	if s.sync == nil {
		return 0
	}
	return s.sync.last
}

// Mismatches is every account the last location at a mismatch found differing
// from the peer's tree, and the block it was located at.
func (s *PulledState) Mismatches() (uint64, []LeafDiff) {
	return s.mismatchAt, s.mismatches
}

// locate walks the peer's BPT as of v against the local tree (#4441), names
// every account whose leaf differs, with both leaves, and owes each to the
// next round, which takes it again; a difference inside a section the node has
// not loaded loads that section. A location that cannot be made, or that finds
// nothing, takes the top of the tree again.
func (s *PulledState) locate(ctx context.Context, p *syncing, v uint64) {
	batch := s.db.Begin(false)
	diffs, peer, err := locateAt(ctx, s.partition, s.partitionSources(ctx), batch.BPT(), v)
	batch.Discard()
	s.mismatchAt, s.mismatches = v, diffs
	if err != nil || len(diffs) == 0 {
		s.log.Warn("The accounts that differ at a block could not be located; taking the top of the tree again",
			"partition", s.partition, "block", v, "error", err)
		p.topTaken = false
		return
	}
	named := 0
	for _, d := range diffs {
		if d.Account != nil || s.openDiff(p, d.KeyHash) {
			named++
		}
	}
	if named == 0 {
		s.log.Warn("The accounts that differ at a block could not be named; taking the top of the tree again",
			"partition", s.partition, "block", v, "differ", len(diffs))
		p.topTaken = false
	}
	for _, d := range diffs {
		s.log.Warn("An account differs from the peer's as of the block the state is complete at",
			"partition", s.partition, "block", v, "peer", peer, "account", d.Account,
			"key", fmt.Sprintf("%x", d.KeyHash), "local", fmt.Sprintf("%x", d.Local), "peer-leaf", fmt.Sprintf("%x", d.Peer))
		if d.Account != nil {
			p.retry[accountKey(d.Account)] = d.Account
		}
	}
}
